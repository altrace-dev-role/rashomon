//go:build unix

package supervise

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Supported reports whether hook entries run supervised on this platform.
const Supported = true

// reapGrace bounds the wait for a killed child to be reaped, and for a
// child's pipes to reach end of file once it has ended. A process the kernel
// cannot finish killing in that time is left for init to reap; the failure
// is recorded without waiting for it.
const reapGrace = 50 * time.Millisecond

// relayedSignals are passed on to the child, which handles them as an
// unsupervised entry would: a controlled exit that closes what it opened.
var relayedSignals = [...]os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}

// Run supervises one hook entry and returns the parent's exit status, which
// is 0 (exitStatus). It never runs the entry's logic itself, not even when
// the child cannot be started: resource pressure is exactly when a fatal
// exit 2 would happen.
func Run(cfg Config) int {
	if cfg.HardStop <= 0 {
		cfg.HardStop = longBudget - longMargin
	}
	var r result
	if err := safe.Guard(func() error { r = supervise(cfg); return nil }); err != nil {
		// A panic in the parent is a bug in the parent; the child, if one
		// was started, stands down when the pipe closes at exit.
		r = result{outcome: store.OutcomeExitNonZero, childExit: -1}
	}
	return exitStatus(r)
}

// result is how one supervised invocation ended.
type result struct {
	outcome   string // empty when the child exited 0 and its output, if any, was forwarded
	childExit int
	signal    string
}

// exitStatus is the parent's exit status: 0, always. It is the seam for the
// one scoped exception the hook output rules allow, a compat PreToolUse
// under Cursor whose child signalled it on the inherited pipe and then exited
// 1 with empty stdout; that needs a child-to-parent channel, and Cursor
// support, neither of which exists yet. Until then a child's exit 1, like any
// other non-zero exit, becomes 0.
func exitStatus(result) int { return 0 }

func supervise(cfg Config) result {
	sigs := make(chan os.Signal, len(relayedSignals)+1)
	// SIGPIPE is caught so that a write to a closed stdout or stderr returns
	// an error instead of ending the parent by signal.
	signal.Notify(sigs, append(relayedSignals[:], syscall.SIGPIPE)...)
	defer signal.Stop(sigs)

	deadline := cfg.Start.Add(cfg.HardStop)
	p := &pump{headDone: make(chan struct{})}

	c, err := spawn(cfg)
	if err != nil {
		// Drain stdin, so the harness's write completes, until it ends or
		// the hard stop comes; a payload still arriving then is cut off by
		// the parent's exit.
		drained := safe.Go(func() { p.run(cfg.Stdin, nil, 0) }, nil)
		waitUntil(drained, deadline)
		r := result{outcome: store.OutcomeSpawnFailed, childExit: -1}
		cfg.fail(p, r, store.ReasonSupervisorSpawnFailed, deadline)
		cfg.observe(nil, r)
		return r
	}

	// The pump is not waited for: a child that has ended needs no more of
	// its input, and the parent's exit ends the pump's last read.
	safe.Go(func() { p.run(cfg.Stdin, c.stdin, cfg.StdinCap) }, nil)
	out := &capped{max: StdoutCap}
	outDone := safe.Go(func() { out.run(c.stdout) }, nil)
	errs := &capped{max: stderrCap}
	errDone := safe.Go(func() { errs.run(c.stderr) }, nil)
	waited := make(chan error, 1)
	safe.Go(func() { waited <- c.cmd.Wait() }, func(err error) {
		select {
		case waited <- err:
		default:
		}
	})

	werr, killed, reaped := c.await(sigs, waited, deadline)
	// A reader that has not reached end of file is still writing its
	// buffer, which is then not read at all: its output is refused and its
	// stderr not relayed.
	outRead := waitUntil(outDone, time.Now().Add(reapGrace))
	errRead := waitUntil(errDone, time.Now().Add(reapGrace))

	r := c.ending(werr, killed, reaped)
	switch {
	case r.outcome != "":
		cfg.fail(p, r, store.ReasonHookKilled, deadline)
	default:
		if errRead {
			cfg.relayStderr(errs.bytes())
		}
		if !outRead || !cfg.forward(p, out, deadline) {
			r.outcome = store.OutcomeOutputRefused
			cfg.fail(p, r, "", deadline)
		}
	}
	cfg.observe(c, r)
	// The clock pipe's write end stays open until here, whatever the
	// collector thinks of c.
	runtime.KeepAlive(c.clock)
	return r
}

// child is a started child and the parent's ends of its pipes.
type child struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
	stderr *os.File
	// clock is the write end of the clock pipe. It is held open until the
	// parent exits, and its end of file is the child's sign that the parent
	// has gone.
	clock *os.File
}

// spawn starts the child: this binary, Token, the entry's arguments, in its
// own process group, with its stdin, stdout and stderr on pipes and the
// clock pipe at fd 3, already holding the clock record.
func spawn(cfg Config) (*child, error) {
	if err := cfg.BeforeSpawn(); err != nil {
		return nil, err
	}
	exe, err := cfg.Executable()
	if err != nil {
		return nil, err
	}
	var opened []*os.File
	closeAll := func() {
		for _, f := range opened {
			f.Close() //nolint:errcheck // the spawn has failed already; this is cleanup
		}
	}
	pipe := func() (r, w *os.File, err error) {
		r, w, err = os.Pipe()
		if err == nil {
			opened = append(opened, r, w)
		}
		return r, w, err
	}
	clockR, clockW, err := pipe()
	if err != nil {
		return nil, err
	}
	if _, err := clockW.Write(clockRecord(cfg.Start, cfg.HardStop)); err != nil {
		closeAll()
		return nil, err
	}
	inR, inW, err := pipe()
	if err != nil {
		closeAll()
		return nil, err
	}
	outR, outW, err := pipe()
	if err != nil {
		closeAll()
		return nil, err
	}
	errR, errW, err := pipe()
	if err != nil {
		closeAll()
		return nil, err
	}

	cmd := exec.Command(exe, append([]string{Token}, cfg.Args...)...)
	// Files, not readers and writers: given an *os.File, exec starts no
	// copying goroutine of its own, so every goroutine here is under
	// safe.Go.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW
	cmd.ExtraFiles = []*os.File{clockR}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		closeAll()
		return nil, err
	}
	// The child holds its own copies of these now; the parent keeps only its
	// ends, so each pipe reaches end of file when the other side is done.
	for _, f := range []*os.File{clockR, inR, outW, errW} {
		f.Close() //nolint:errcheck // the child's copy is what matters
	}
	return &child{cmd: cmd, stdin: inW, stdout: outR, stderr: errR, clock: clockW}, nil
}

// await waits for the child to end, relaying signals to it, and kills its
// process group at the hard stop. reaped is false only when a killed child
// was not reaped within reapGrace.
func (c *child) await(sigs <-chan os.Signal, waited <-chan error, deadline time.Time) (werr error, killed, reaped bool) {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	pid := c.cmd.Process.Pid
	for {
		select {
		case werr = <-waited:
			return werr, false, true
		case s := <-sigs:
			if sig, ok := s.(syscall.Signal); ok && sig != syscall.SIGPIPE {
				relay(pid, sig)
			}
		case <-timer.C:
			// An end that arrived with the timer is an end, not a kill.
			select {
			case werr = <-waited:
				return werr, false, true
			default:
			}
			killGroup(pid)
			select {
			case werr = <-waited:
				return werr, true, true
			case <-time.After(reapGrace):
				return nil, true, false
			}
		}
	}
}

// relay passes a signal on to the child.
func relay(pid int, sig syscall.Signal) {
	syscall.Kill(pid, sig) //nolint:errcheck // ESRCH: the child has ended, and Wait is reporting it
}

// killGroup kills the child's process group, the child with anything it
// started. The child leads the group and is not yet reaped when this runs
// (await checks first), so its pid still names the group; ESRCH means the
// group had already gone.
func killGroup(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck // ESRCH: the group has gone already
}

// ending reads how the child ended into a result: an empty outcome for an
// exit 0, a failure otherwise.
func (c *child) ending(werr error, killed, reaped bool) result {
	r := result{childExit: -1}
	var ws syscall.WaitStatus
	have := false
	if reaped && c.cmd.ProcessState != nil {
		ws, have = c.cmd.ProcessState.Sys().(syscall.WaitStatus)
	}
	switch {
	case killed:
		r.outcome = store.OutcomeKilledAtHardStop
		r.signal = signalName(syscall.SIGKILL)
	case !have:
		// Wait failed without a status: the child's end is unknown, which
		// is a failure, never a pass.
		r.outcome = store.OutcomeExitNonZero
	case ws.Signaled():
		r.outcome = store.OutcomeSignalled
		r.signal = signalName(ws.Signal())
	case ws.ExitStatus() != 0:
		r.outcome = store.OutcomeExitNonZero
		r.childExit = ws.ExitStatus()
	default:
		r.childExit = 0
		if werr != nil && !errors.As(werr, new(*exec.ExitError)) {
			r.outcome = store.OutcomeExitNonZero
		}
	}
	return r
}

// fail records a failed entry and, for a child that failed, was killed or
// could not be started, writes the fixed stderr line. A refused output is
// recorded with no stderr line and no coverage reason.
func (cfg Config) fail(p *pump, r result, reason string, deadline time.Time) {
	// The ids come from the head the pump kept; wait a moment for it if the
	// child ended before the first 4 KiB had arrived.
	waitUntil(p.headDone, minTime(deadline, time.Now().Add(reapGrace)))
	h := scanHead(p.headBytes())
	recorded := cfg.Record(Failure{
		Reason:    reason,
		Outcome:   r.outcome,
		SessionID: h.SessionID,
		Event:     eventOf(cfg.Events, h.Event),
		ChildExit: r.childExit,
		Signal:    r.signal,
	})
	if reason == "" {
		return
	}
	cfg.say(InternalErrorLine)
	if !recorded {
		cfg.say(RecordLostLine)
	}
}

// forward writes a child's output to stdout when the gate allows it, and
// reports false when it refused it. Nothing is written for an empty one.
func (cfg Config) forward(p *pump, out *capped, deadline time.Time) bool {
	if out.over {
		return false
	}
	b := bytes.TrimSpace(out.bytes())
	if len(b) == 0 {
		return true
	}
	waitUntil(p.headDone, minTime(deadline, time.Now().Add(reapGrace)))
	event := eventOf(cfg.Events, scanHead(p.headBytes()).Event)
	if !Forwardable(event, b) {
		return false
	}
	if cfg.BeforeStdout != nil {
		cfg.BeforeStdout()
	}
	cfg.Stdout.Write(b) //nolint:errcheck // a failed write means Claude Code stopped reading; no one is left to tell
	return true
}

// relayStderr passes on the fixed lines of a child that exited 0.
func (cfg Config) relayStderr(b []byte) {
	for len(b) > 0 {
		line, rest, found := bytes.Cut(b, []byte{'\n'})
		if !found {
			return // a line the cap cut short is not a fixed line
		}
		if cfg.Relayed(string(line)) {
			cfg.say(string(line))
		}
		b = rest
	}
}

// say writes one line to stderr.
func (cfg Config) say(line string) {
	io.WriteString(cfg.Stderr, line+"\n") //nolint:errcheck // a failed write has no one to report to
}

// observe hands the invocation's figures to Observe, when it is set.
func (cfg Config) observe(c *child, r result) {
	if cfg.Observe == nil {
		return
	}
	s := Stats{Outcome: r.outcome, ParentMaxRSS: selfMaxRSS()}
	if c != nil {
		s.ChildPID = c.cmd.Process.Pid
		if ps := c.cmd.ProcessState; ps != nil {
			if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
				s.ChildMaxRSS = rssBytes(ru.Maxrss)
			}
		}
	}
	cfg.Observe(s)
}

// rusageMaxRSS is getrusage's peak resident set for this process, in bytes.
// On Linux it also counts the image exec replaced (selfMaxRSS).
func rusageMaxRSS() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return rssBytes(ru.Maxrss)
}

// rssBytes converts ru_maxrss to bytes: darwin reports bytes, the others
// kilobytes.
func rssBytes(v int64) int64 {
	if runtime.GOOS == "darwin" {
		return v
	}
	return v << 10
}

// pump streams the payload to the child, keeping its first 4 KiB.
type pump struct {
	mu   sync.Mutex
	head [headBytes]byte
	n    int
	// headDone is closed when the head is full or the payload has ended.
	headDone chan struct{}
	headOnce sync.Once
}

// run copies in to out, at most capBytes of it, then closes out and reads
// the rest of in to its end. A nil out, or one the child has closed, only
// drains.
func (p *pump) run(in io.Reader, out *os.File, capBytes int64) {
	defer p.headOnce.Do(func() { close(p.headDone) })
	buf := make([]byte, pumpBuffer)
	var sent int64
	for {
		n, err := in.Read(buf)
		if n > 0 {
			p.keep(buf[:n])
			if out != nil {
				k := min(int64(n), capBytes-sent)
				if _, werr := out.Write(buf[:k]); werr != nil {
					// The child has closed its stdin or ended: drain.
					out = closed(out)
				} else if sent += k; sent >= capBytes {
					// The cap: the child sees end of file here and
					// treats the payload as over the limit.
					out = closed(out)
				}
			}
		}
		if err != nil {
			break
		}
	}
	if out != nil {
		closed(out)
	}
}

// closed closes the child's stdin and returns nil, the pump's "draining".
func closed(f *os.File) *os.File {
	f.Close() //nolint:errcheck // the child reads end of file either way
	return nil
}

// keep copies what fits of b into the head.
func (p *pump) keep(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.n == headBytes {
		return
	}
	p.n += copy(p.head[p.n:], b)
	if p.n == headBytes {
		p.headOnce.Do(func() { close(p.headDone) })
	}
}

// headBytes is a copy of what the head holds now.
func (p *pump) headBytes() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.head[:p.n]...)
}

// capped reads a pipe to its end, keeping the first max bytes.
type capped struct {
	max  int
	buf  []byte
	over bool
}

func (c *capped) run(f *os.File) {
	defer f.Close() //nolint:errcheck // read to its end; nothing to flush
	var scratch [4 << 10]byte
	for {
		n, err := f.Read(scratch[:])
		if n > 0 {
			if len(c.buf)+n > c.max {
				c.over = true
				c.buf = append(c.buf, scratch[:min(n, c.max-len(c.buf))]...)
			} else {
				c.buf = append(c.buf, scratch[:n]...)
			}
		}
		if err != nil {
			return
		}
	}
}

// bytes is what was kept. It is read only after run has returned.
func (c *capped) bytes() []byte { return c.buf }

// waitUntil waits for done or until t, whichever is first, and reports
// whether done was.
func waitUntil(done <-chan struct{}, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// signalName names the signals a child can plausibly end by; any other is
// "signal" and its number is not kept, so the line stays a code.
func signalName(s syscall.Signal) string {
	switch s {
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGABRT:
		return "SIGABRT"
	case syscall.SIGSEGV:
		return "SIGSEGV"
	case syscall.SIGBUS:
		return "SIGBUS"
	case syscall.SIGPIPE:
		return "SIGPIPE"
	}
	return "signal"
}

// Child is the supervised child's view of its parent: the clock they share,
// and the pipe whose end of file says the parent has gone.
type Child struct {
	clock budget.Clock
	gone  atomic.Bool
}

// Attach recognises the supervised child: Token first in args, and at fd 3 a
// pipe holding one whole clock record. It returns the arguments without the
// token either way. Without the pipe and its record the invocation is not
// the child, whatever its argv says, and is supervised like any other.
func Attach(args []string) (c *Child, rest []string, ok bool) {
	if len(args) == 0 || args[0] != Token {
		return nil, args, false
	}
	rest = args[1:]
	var st syscall.Stat_t
	if err := syscall.Fstat(clockFD, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFIFO {
		return nil, rest, false
	}
	// Non-blocking, so every later look at the pipe is a poll. Read with
	// the raw call, not an *os.File, which would park on the runtime's
	// poller until the pipe was readable -- the wait this must not do.
	if err := syscall.SetNonblock(clockFD, true); err != nil {
		return nil, rest, false
	}
	var buf [maxClockRecord]byte
	n := 0
	for n < len(buf) {
		k, err := syscall.Read(clockFD, buf[n:])
		if err != nil || k <= 0 {
			break
		}
		n += k
		if buf[n-1] == '\n' {
			break
		}
	}
	start, stop, valid := parseClockRecord(buf[:n])
	if !valid {
		return nil, rest, false
	}
	clock, err := budget.NewClock(start, stop)
	if err != nil {
		return nil, rest, false
	}
	return &Child{clock: clock}, rest, true
}

// SoftClock returns e's soft-deadline Clock, sharing the parent's process
// start that c's own hard-stop clock already carries -- so a phase's soft
// deadline counts from when the parent started, not from whenever the
// phase happens to ask for a Clock. A nil Child is an unsupervised process
// with no parent start to share, so its Clock starts now.
func (c *Child) SoftClock(e budget.DeadlineEntry) budget.Clock {
	if c == nil {
		return budget.NewClock(time.Now(), e.SoftBudget())
	}
	return c.clock.Soft(e.SoftBudget())
}

// StandDown reports whether the child must start nothing new: its parent has
// gone, or its hard stop has passed. A nil Child is an unsupervised process,
// which never stands down.
func (c *Child) StandDown() bool {
	if c == nil {
		return false
	}
	return c.parentGone() || c.clock.Expired(time.Now())
}

// parentGone polls the clock pipe: end of file means every write end has
// closed, and the parent held the only one.
func (c *Child) parentGone() bool {
	if c.gone.Load() {
		return true
	}
	var b [1]byte
	n, err := syscall.Read(clockFD, b[:])
	if n == 0 && err == nil {
		c.gone.Store(true)
	}
	// EAGAIN: the parent is there. EINTR or anything else: ask again next
	// time.
	return c.gone.Load()
}
