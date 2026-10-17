//go:build unix

package supervise

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The test binary is its own child: started with Token first, TestMain runs
// helper instead of the tests. Its mode is the first argument after Token.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == Token {
		os.Exit(helper(os.Args[1:]))
	}
	// A race-built helper sleeps a second at exit by default; none of these
	// tests is about that second.
	if err := os.Setenv("GORACE", "atexit_sleep_ms=0"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

const fixedLine = "rashomon: entry belongs to another install; standing down"

// helper is the child side of these tests.
func helper(args []string) int {
	c, rest, attached := Attach(args)
	mode := ""
	if len(rest) > 0 {
		mode = rest[0]
	}
	arg := func(i int) string {
		if len(rest) > i {
			return rest[i]
		}
		return ""
	}
	switch mode {
	case "attach-check":
		fmt.Printf(`{"systemMessage":"attached=%v"}`, attached)
		return 0
	case "exit0":
		io.Copy(io.Discard, os.Stdin) //nolint:errcheck // test helper
		return 0
	case "exit1":
		return 1
	case "print":
		io.Copy(io.Discard, os.Stdin) //nolint:errcheck // test helper
		os.Stdout.WriteString(arg(1)) //nolint:errcheck // test helper
		if arg(2) != "" {
			n, _ := strconv.Atoi(arg(2))
			return n
		}
		return 0
	case "big":
		os.Stdout.Write(bytes.Repeat([]byte("x"), StdoutCap+1)) //nolint:errcheck // test helper
		return 0
	case "count":
		n, _ := io.Copy(io.Discard, os.Stdin)
		fmt.Printf(`{"systemMessage":"%d"}`, n)
		return 0
	case "stderr":
		fmt.Fprintln(os.Stderr, fixedLine)
		fmt.Fprintln(os.Stderr, "panic: payload text that must not leak")
		fmt.Fprint(os.Stderr, fixedLine) // no newline: cut short, not relayed
		n, _ := strconv.Atoi(arg(1))
		return n
	case "fatal":
		debug.SetMaxStack(64 << 10)
		var recurse func(int) int
		recurse = func(n int) int { var pad [512]byte; pad[n%512] = 1; return recurse(n+1) + int(pad[0]) }
		return recurse(0)
	case "hang":
		// A grandchild in the same group, its pid written where the test
		// can find it, then wait to be killed.
		g := exec.Command("/bin/sleep", "30")
		if err := g.Start(); err != nil {
			return 9
		}
		os.WriteFile(arg(1), []byte(strconv.Itoa(g.Process.Pid)), 0o600) //nolint:errcheck // test helper
		time.Sleep(30 * time.Second)
		return 0
	case "maxrss":
		fmt.Print(selfMaxRSS())
		return 0
	case "standdown":
		// Poll as an entry does between its phases; say why it stood down.
		for i := 0; i < 2000; i++ {
			if c.StandDown() {
				why := "stop"
				if c.parentGone() {
					why = "gone"
				}
				os.WriteFile(arg(1), []byte(why), 0o600) //nolint:errcheck // test helper
				return 0
			}
			time.Sleep(5 * time.Millisecond)
		}
		os.WriteFile(arg(1), []byte("never"), 0o600) //nolint:errcheck // test helper
		return 0
	}
	return 7
}

// rig runs Run in this process against the helper.
type rig struct {
	mu       sync.Mutex
	failures []Failure
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	stdin    io.Reader
	stop     time.Duration
	cap      int64
	spawnErr error
	events   []string
}

func newRig() *rig {
	return &rig{stdin: strings.NewReader(`{"session_id":"sess-1","hook_event_name":"Stop"}`), stop: 5 * time.Second, cap: 8<<20 + 1, events: []string{"Stop", "StopFailure"}}
}

func (r *rig) run(t *testing.T, args ...string) (int, time.Duration) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	code := Run(Config{
		Args: args, Events: r.events, Start: start, HardStop: r.stop, StdinCap: r.cap,
		Stdin: r.stdin, Stdout: &r.stdout, Stderr: &r.stderr,
		Executable:  func() (string, error) { return exe, nil },
		BeforeSpawn: func() error { return r.spawnErr },
		Relayed:     func(line string) bool { return line == fixedLine },
		Record: func(f Failure) bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.failures = append(r.failures, f)
			return true
		},
	})
	return code, time.Since(start)
}

func (r *rig) failure(t *testing.T) Failure {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.failures) != 1 {
		t.Fatalf("%d failures recorded, want 1: %+v", len(r.failures), r.failures)
	}
	return r.failures[0]
}

func (r *rig) noFailure(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.failures) != 0 {
		t.Fatalf("failures recorded for a clean child: %+v", r.failures)
	}
}

// TestSTR4_AFatalChildExitsZeroAndPrintsNothing: the child dies of a
// runtime fatal error, exit 2; the parent exits 0, prints nothing to stdout,
// writes the one fixed stderr line, and records hook_killed with the ids it
// read from the payload.
func TestSTR4_AFatalChildExitsZeroAndPrintsNothing(t *testing.T) {
	r := newRig()
	code, _ := r.run(t, "fatal")
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	if r.stdout.Len() != 0 {
		t.Errorf("stdout %q, want nothing", r.stdout.String())
	}
	if got := r.stderr.String(); got != InternalErrorLine+"\n" {
		t.Errorf("stderr %q, want the fixed line alone", got)
	}
	f := r.failure(t)
	if f.Reason != store.ReasonHookKilled || f.Outcome != store.OutcomeExitNonZero || f.ChildExit != 2 {
		t.Errorf("recorded %+v, want hook_killed / exit_nonzero / 2", f)
	}
	if f.SessionID != "sess-1" || f.Event != "Stop" {
		t.Errorf("recorded ids %q %q, want sess-1 Stop", f.SessionID, f.Event)
	}
}

// TestG6_ExitOneStillBecomesZero pins today's behaviour at the seam for the
// one scoped exit-1 exception: without it, a child's exit 1 is a failure
// like any other and the parent exits 0.
func TestG6_ExitOneStillBecomesZero(t *testing.T) {
	r := newRig()
	if code, _ := r.run(t, "exit1"); code != 0 {
		t.Errorf("exit %d after a child's exit 1, want 0", code)
	}
	if f := r.failure(t); f.Outcome != store.OutcomeExitNonZero || f.ChildExit != 1 {
		t.Errorf("recorded %+v, want exit_nonzero / 1", f)
	}
}

// TestSTR6_AHungChildIsKilledWithItsGroupAtTheHardStop: a child still
// running at the hard stop is killed with everything in its process group,
// and the parent returns promptly after.
func TestSTR6_AHungChildIsKilledWithItsGroupAtTheHardStop(t *testing.T) {
	r := newRig()
	r.stop = 400 * time.Millisecond
	pidFile := filepath.Join(t.TempDir(), "grandchild")
	code, took := r.run(t, "hang", pidFile)
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	if took > r.stop+500*time.Millisecond {
		t.Errorf("Run took %v against a %v hard stop", took, r.stop)
	}
	if f := r.failure(t); f.Reason != store.ReasonHookKilled || f.Outcome != store.OutcomeKilledAtHardStop || f.Signal != "SIGKILL" {
		t.Errorf("recorded %+v, want hook_killed / killed_at_hard_stop / SIGKILL", f)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the child never started its grandchild: %v", err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	waitGone(t, pid)
	if r.stdout.Len() != 0 {
		t.Errorf("stdout %q after a kill", r.stdout.String())
	}
}

// waitGone requires pid to stop existing within a second. The grandchild is
// reparented once its parent dies, and init reaps it.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // cleanup of a survivor
	t.Errorf("pid %d outlived the hard stop's group kill", pid)
}

// TestSTR4h_OutputIsForwardedOnlyFromACleanExit: a child's output reaches
// stdout only when the child exited 0 and the output passes the gate; a
// refused output prints nothing and is recorded without a coverage reason
// or a stderr line.
func TestSTR4h_OutputIsForwardedOnlyFromACleanExit(t *testing.T) {
	const line = `{"systemMessage":"rashomon: 3 calls"}`
	t.Run("clean", func(t *testing.T) {
		r := newRig()
		r.run(t, "print", line)
		if r.stdout.String() != line {
			t.Errorf("stdout %q, want %q", r.stdout.String(), line)
		}
		r.noFailure(t)
		if r.stderr.Len() != 0 {
			t.Errorf("stderr %q", r.stderr.String())
		}
	})
	t.Run("exit 1", func(t *testing.T) {
		r := newRig()
		r.run(t, "print", line, "1")
		if r.stdout.Len() != 0 {
			t.Errorf("a failed child's output was forwarded: %q", r.stdout.String())
		}
		if f := r.failure(t); f.Reason != store.ReasonHookKilled {
			t.Errorf("recorded %+v", f)
		}
	})
	for _, c := range []struct{ name, out string }{
		{"junk", "rashomon: hello"},
		{"a decision on Stop", `{"hookSpecificOutput":{"hookEventName":"Stop","permissionDecision":"deny","permissionDecisionReason":"r"}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig()
			r.run(t, "print", c.out)
			if r.stdout.Len() != 0 {
				t.Errorf("refused output was forwarded: %q", r.stdout.String())
			}
			if f := r.failure(t); f.Reason != "" || f.Outcome != store.OutcomeOutputRefused || f.ChildExit != 0 {
				t.Errorf("recorded %+v, want no reason / output_refused / 0", f)
			}
			if r.stderr.Len() != 0 {
				t.Errorf("stderr %q for a refused output", r.stderr.String())
			}
		})
	}
	t.Run("over the cap", func(t *testing.T) {
		r := newRig()
		r.run(t, "big")
		if r.stdout.Len() != 0 {
			t.Errorf("%d bytes forwarded past the %d-byte cap", r.stdout.Len(), StdoutCap)
		}
		if f := r.failure(t); f.Outcome != store.OutcomeOutputRefused {
			t.Errorf("recorded %+v", f)
		}
	})
}

// TestRES8b_TheChildIsGivenAtMostTheCap: the parent streams the payload and
// cuts the child's stdin at the cap, so the child reads exactly that much.
func TestRES8b_TheChildIsGivenAtMostTheCap(t *testing.T) {
	r := newRig()
	r.cap = 1000
	r.stdin = io.MultiReader(strings.NewReader(`{"session_id":"s","hook_event_name":"Stop","x":"`), bytes.NewReader(bytes.Repeat([]byte("y"), 100<<10)))
	r.run(t, "count")
	if want := `{"systemMessage":"1000"}`; r.stdout.String() != want {
		t.Errorf("child read %q, want %q", r.stdout.String(), want)
	}
	r.noFailure(t)
}

// TestSTR4i_OnlyTheFixedStderrLinesPass: of a child's stderr, a fixed line
// passes when the child exited 0, and nothing else ever does.
func TestSTR4i_OnlyTheFixedStderrLinesPass(t *testing.T) {
	r := newRig()
	r.run(t, "stderr", "0")
	if got := r.stderr.String(); got != fixedLine+"\n" {
		t.Errorf("stderr %q, want only the fixed line", got)
	}
	r = newRig()
	r.run(t, "stderr", "3")
	if got := r.stderr.String(); got != InternalErrorLine+"\n" {
		t.Errorf("stderr %q after a failed child, want only the internal-error line", got)
	}
}

// TestSTR4c_ASpawnFailureDrainsStdinAndRecords: a child that cannot be
// started leaves the payload read to its end, nothing on stdout, and a
// supervisor_spawn_failed record carrying the ids.
func TestSTR4c_ASpawnFailureDrainsStdinAndRecords(t *testing.T) {
	r := newRig()
	r.spawnErr = syscall.EAGAIN
	payload := bytes.NewReader(append([]byte(`{"session_id":"sess-9","hook_event_name":"Stop","x":"`), bytes.Repeat([]byte("y"), 1<<20)...))
	r.stdin = payload
	if code, _ := r.run(t, "exit0"); code != 0 {
		t.Errorf("exit %d", code)
	}
	if payload.Len() != 0 {
		t.Errorf("%d bytes of stdin left unread", payload.Len())
	}
	if r.stdout.Len() != 0 {
		t.Errorf("stdout %q", r.stdout.String())
	}
	f := r.failure(t)
	if f.Reason != store.ReasonSupervisorSpawnFailed || f.Outcome != store.OutcomeSpawnFailed || f.ChildExit != -1 || f.SessionID != "sess-9" {
		t.Errorf("recorded %+v", f)
	}
}

// fakeParent starts the helper as the supervisor would, with the clock pipe
// at fd 3 holding record, and returns the write end, still open.
func fakeParent(t *testing.T, record []byte, args ...string) (*exec.Cmd, *os.File) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(record); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, append([]string{Token}, args...)...)
	cmd.ExtraFiles = []*os.File{r}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r.Close() //nolint:errcheck // the child has its copy
	t.Cleanup(func() {
		cmd.Process.Kill() //nolint:errcheck // cleanup
		cmd.Wait()         //nolint:errcheck // cleanup
	})
	return cmd, w
}

// TestSTR6d_TheChildStandsDownWhenItsParentGoes: closing the parent's end of
// the clock pipe is the parent going; the child's next poll stands it down,
// well before its own hard stop.
func TestSTR6d_TheChildStandsDownWhenItsParentGoes(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "why")
	cmd, w := fakeParent(t, clockRecord(time.Now(), time.Minute), "standdown", marker)
	time.Sleep(50 * time.Millisecond)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "gone" {
		t.Errorf("the child stood down for %q, want gone", b)
	}
}

// TestSTR6e_TheChildStandsDownAtItsOwnHardStop: with its parent still
// there, the child stands down when the shared clock's hard stop passes.
func TestSTR6e_TheChildStandsDownAtItsOwnHardStop(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "why")
	start := time.Now()
	cmd, w := fakeParent(t, clockRecord(start, 300*time.Millisecond), "standdown", marker)
	defer w.Close() //nolint:errcheck // held open on purpose until the child is done
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "stop" {
		t.Errorf("the child stood down for %q, want stop", b)
	}
	if took := time.Since(start); took < 300*time.Millisecond {
		t.Errorf("the child stood down after %v, before its 300ms stop", took)
	}
}

// TestTRU50_ArgvAloneDoesNotMakeAChild: the token is not enough. Without the
// pipe at fd 3, with a regular file there holding a valid record, or with a
// pipe holding something else, Attach refuses.
func TestTRU50_ArgvAloneDoesNotMakeAChild(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, extra *os.File) string {
		t.Helper()
		cmd := exec.Command(exe, Token, "attach-check")
		if extra != nil {
			cmd.ExtraFiles = []*os.File{extra}
		}
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	t.Run("nothing at fd 3", func(t *testing.T) {
		if got := check(t, nil); !strings.Contains(got, "attached=false") {
			t.Errorf("%s", got)
		}
	})
	t.Run("a regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "clock")
		if err := os.WriteFile(path, clockRecord(time.Now(), time.Minute), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close() //nolint:errcheck // test cleanup
		if got := check(t, f); !strings.Contains(got, "attached=false") {
			t.Errorf("%s", got)
		}
	})
	t.Run("a pipe with junk", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close() //nolint:errcheck // test cleanup
		if _, err := w.WriteString("hello\n"); err != nil {
			t.Fatal(err)
		}
		got := check(t, r)
		r.Close() //nolint:errcheck // test cleanup
		if !strings.Contains(got, "attached=false") {
			t.Errorf("%s", got)
		}
	})
	t.Run("the supervisor's pipe", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close() //nolint:errcheck // test cleanup
		if _, err := w.Write(clockRecord(time.Now(), time.Minute)); err != nil {
			t.Fatal(err)
		}
		got := check(t, r)
		r.Close() //nolint:errcheck // test cleanup
		if !strings.Contains(got, "attached=true") {
			t.Errorf("the real pipe was refused: %s", got)
		}
	})
}

// TestRES8c_TheParentsOwnPeakLeavesOutTheImageExecReplaced: the figure the
// supervisor reports for itself is its own image's peak. Linux folds the
// peak of the image exec replaced -- the forked copy of whatever started it
// -- into getrusage's maxrss, so a supervisor started by a large process
// would read as large as that process; the fixture is a 96 MiB parent.
func TestRES8c_TheParentsOwnPeakLeavesOutTheImageExecReplaced(t *testing.T) {
	ballast := make([]byte, 96<<20)
	for i := range ballast {
		if i%4096 == 0 {
			ballast[i] = 1
		}
	}
	out, err := exec.Command(os.Args[0], Token, "maxrss").Output()
	if err != nil {
		t.Fatal(err)
	}
	peak, err := strconv.ParseInt(string(out), 10, 64)
	if err != nil || peak <= 0 {
		t.Fatalf("the helper printed %q", out)
	}
	if peak >= 48<<20 {
		t.Errorf("the helper's own peak reads %.1f MiB beside a 96 MiB parent: it is not its own", float64(peak)/(1<<20))
	}
	runtime.KeepAlive(ballast)
}
