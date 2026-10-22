package hook

import (
	"errors"
	"fmt"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRubricERRORS8_CloseReasonNamesARecoveredPanic: a capture that ended in
// a recovered panic is recorded as recovered_panic, a code of its own, and an
// ordinary failure stays internal_error. Before, both were internal_error and
// a reader could not tell a bug in this program from a failed write.
func TestRubricERRORS8_CloseReasonNamesARecoveredPanic(t *testing.T) {
	recovered := safe.Guard(func() error { panic("boom") })
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"a recovered panic", recovered, store.ReasonRecoveredPanic},
		{"a wrapped recovered panic", fmt.Errorf("capture: %w", recovered), store.ReasonRecoveredPanic},
		{"an ordinary failure", errors.New("disk full"), store.ReasonInternalError},
		{"a lock timeout", store.ErrLockTimeout, store.ReasonLockTimeout},
	} {
		if got := closeReason(c.err); got != c.want {
			t.Errorf("%s: closeReason = %q, want %q", c.name, got, c.want)
		}
	}
}
