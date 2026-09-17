//go:build attestfault

package fault

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/safe"
)

// EnvVar selects a fault. Its value is either "<kind>", which fires at the
// first injection point reached, or "<point>:<kind>", which fires only at that
// point.
const EnvVar = "ATTEST_FAULT"

// Fault kinds. Every one of these except Hang panics; none returns an error. A
// fault that returned an error would exercise error handling, not the panic
// barrier, and exit code 1 does not block a tool call.
//
// Hang holds the process open, until a SIGTERM arrives or a bound expires, so
// that the signal paths can be exercised: "hang" waits ten seconds, "hang=N"
// waits N.
const (
	NilMapWrite         = "nil_map_write"
	NilPointerDeref     = "nil_pointer_deref"
	IndexOutOfRange     = "index_out_of_range"
	SendOnClosedChannel = "send_on_closed_channel"
	GoroutinePanic      = "goroutine_panic"
	PlainPanic          = "plain_panic"
	Hang                = "hang"
)

// Enabled reports whether fault injection is compiled in.
func Enabled() bool { return true }

// sink defeats the dead-store elimination that would otherwise let the compiler
// drop a nil dereference whose result is never read.
var sink byte

// Inject fires the configured fault if this is the selected point.
func Inject(point string) {
	spec := os.Getenv(EnvVar)
	if spec == "" {
		return
	}
	kind := spec
	if i := strings.IndexByte(spec, ':'); i >= 0 {
		if spec[:i] != point {
			return
		}
		kind = spec[i+1:]
	}
	trigger(kind)
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
