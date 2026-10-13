//go:build rashomonfault

package fault

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
)

// EnvVar selects a fault. Its value is either "<kind>", which fires at the
// first injection point reached, or "<point>:<kind>", which fires only at that
// point.
const EnvVar = "RASHOMON_FAULT"

// Fault kinds. Every one of these except Hang and FailN panics. A fault that
// returned an error would exercise error handling, not the panic barrier, and
// exit code 1 does not block a tool call.
//
// Hang holds the process open, until a SIGTERM arrives or a bound expires, so
// that the signal paths can be exercised: "hang" waits ten seconds, "hang=N"
// waits N.
//
// FailN is the exception the package comment names: "fail=N" makes the first N
// calls at the selected point return ErrInjected, through Fail. It is here for
// the retry around a transient settings read, which nothing that panics can
// exercise, because the retry never gets to run.
const (
	NilMapWrite         = "nil_map_write"
	NilPointerDeref     = "nil_pointer_deref"
	IndexOutOfRange     = "index_out_of_range"
	SendOnClosedChannel = "send_on_closed_channel"
	GoroutinePanic      = "goroutine_panic"
	PlainPanic          = "plain_panic"
	Hang                = "hang"
	FailN               = "fail"
)

// ErrInjected is what a "fail=N" point returns. One fixed value, because what
// is under test is the caller's response to a failed read and not its reading
// of the message.
var ErrInjected = errors.New("fault: injected failure")

// Enabled reports whether fault injection is compiled in.
func Enabled() bool { return true }

// sink defeats the dead-store elimination that would otherwise let the compiler
// drop a nil dereference whose result is never read.
//
//lint:ignore U1000 written and never read, by design: see above.
var sink byte

// Inject fires the configured fault if this is the selected point.
func Inject(point string) {
	if kind, ok := selected(point); ok {
		trigger(kind)
	}
}

// Fail returns ErrInjected for the first N calls at point when the selected
// kind is "fail=N", and nil after those and everywhere else. It is the one
// injection that returns rather than panics; see the package comment.
func Fail(point string) error {
	kind, ok := selected(point)
	if !ok || !strings.HasPrefix(kind, FailN) {
		return nil
	}
	n := 1
	if _, digits, cut := strings.Cut(kind, "="); cut {
		if v, err := strconv.Atoi(digits); err == nil {
			n = v
		}
	}
	if failures >= n {
		return nil
	}
	failures++
	return ErrInjected
}

// failures counts what Fail has already failed. One counter, because the spec
// selects one point; per process, because a hook invocation is one process and
// "the first N calls" is a count within it.
var failures int

// selected reports the configured kind when point is the selected point.
func selected(point string) (string, bool) {
	spec := os.Getenv(EnvVar)
	if spec == "" {
		return "", false
	}
	kind := spec
	if i := strings.IndexByte(spec, ':'); i >= 0 {
		if spec[:i] != point {
			return "", false
		}
		kind = spec[i+1:]
	}
	return kind, true
}

func trigger(kind string) {
	if strings.HasPrefix(kind, Hang) {
		hang(kind)
		return
	}

	switch kind {
	case NilMapWrite:
		var m map[string]string
		m["injected"] = "fault"

	case NilPointerDeref:
		type payload struct{ b byte }
		var p *payload
		sink = p.b

	case IndexOutOfRange:
		// A truncated payload: the read is past the end of what arrived.
		b := truncate([]byte(`{"tool_name":"Bash","tool_input":{}}`), 1)
		sink = b[len(b)+4]

	case SendOnClosedChannel:
		ch := make(chan int, 1)
		close(ch)
		ch <- 1

	case GoroutinePanic:
		// Waiting on the channel is load-bearing. Without it the process can
		// exit before this goroutine is scheduled, and the assertion that we
		// survived a goroutine panic would pass without one ever happening.
		<-safe.Go(func() { panic("injected goroutine fault") }, nil)

	case PlainPanic:
		panic("injected fault")
	}
}

func hang(kind string) {
	wait := 10 * time.Second
	if _, secs, ok := strings.Cut(kind, "="); ok {
		if n, err := strconv.Atoi(secs); err == nil {
			wait = time.Duration(n) * time.Second
		}
	}

	// Notified alongside main's own handler: both channels receive the signal,
	// so main still sees the cancellation once this returns.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sig)

	select {
	case <-time.After(wait):
	case <-sig:
	}
}

// truncate is not inlined so the compiler cannot fold the length below into a
// constant and reject the out-of-range index at compile time.
//
//go:noinline
func truncate(b []byte, n int) []byte { return b[:n] }

// SyncLogEnvVar names the file the sync recorder appends to: one line per
// sync the store issues, "<kind> <file> locked=<true|false>", in the order
// they happen. Unset, nothing is recorded.
//
// It is REC-34's instrument. Durability cannot be observed without a power
// loss, so its test observes the next best thing: which sync ran, on which
// file, and whether a store lock was held while it ran. "locked" is counted
// from LockTaken and LockReleased, around every lock the store takes, append
// or not, rather than claimed by the code that syncs, so a full sync moved
// back inside a lock is recorded as exactly that. The count is per process,
// so with several goroutines it can only err towards "locked".
const SyncLogEnvVar = "RASHOMON_SYNC_LOG"

// locksHeld counts the store locks this process holds.
var locksHeld atomic.Int32

// LockTaken and LockReleased bracket every store lock.
func LockTaken() { locksHeld.Add(1) }

func LockReleased() { locksHeld.Add(-1) }

// RecordSync logs one sync of kind (SyncBarrier or SyncFull) on path.
func RecordSync(kind, path string) {
	log := os.Getenv(SyncLogEnvVar)
	if log == "" {
		return
	}
	line := kind + " " + filepath.Base(path) + " locked=" + strconv.FormatBool(locksHeld.Load() > 0) + "\n"
	if err := appendLog(log, line); err != nil {
		// A line the recorder could not write would read as a sync that never
		// happened. stderr is what turns that into a failing test instead of
		// a passing one: every test that sets the variable requires it empty.
		fmt.Fprintln(os.Stderr, "rashomon: sync recorder:", err)
	}
}

func appendLog(path, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line); err != nil {
		f.Close() //nolint:errcheck // the write error is what matters
		return err
	}
	return f.Close()
}
