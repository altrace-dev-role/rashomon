//go:build attestfault

package fault

import (
	"os"
	"strings"

	"github.com/altrace-dev-role/altrace-attest/internal/safe"
)

// EnvVar selects a fault. Its value is either "<kind>", which fires at the
// first injection point reached, or "<point>:<kind>", which fires only at that
// point.
const EnvVar = "ATTEST_FAULT"

// Fault kinds. Every one of these panics; none of them returns an error. That
// is the point -- a fault that returns an error exercises error handling, not
// the panic barrier, and exit code 1 does not block a tool call.
const (
	NilMapWrite         = "nil_map_write"
	NilPointerDeref     = "nil_pointer_deref"
	IndexOutOfRange     = "index_out_of_range"
	SendOnClosedChannel = "send_on_closed_channel"
	GoroutinePanic      = "goroutine_panic"
	PlainPanic          = "plain_panic"
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

// truncate is not inlined so the compiler cannot fold the length below into a
// constant and reject the out-of-range index at compile time.
//
//go:noinline
func truncate(b []byte, n int) []byte { return b[:n] }
