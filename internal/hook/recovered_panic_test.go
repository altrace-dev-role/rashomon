package hook

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

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

// TestRubricERRORS8_RecoveredLabelPanicIsACoverageReason: a panic recovered
// inside the file-label derivation costs the label, which reads unknown, and
// the call's coverage record says a panic was recovered. The declaration and
// its terminal stand: the call itself was captured. Before, the only trace was
// LabelUnknown, the same value a path with no recognisable name gets.
func TestRubricERRORS8_RecoveredLabelPanicIsACoverageReason(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	st, err := store.Open(home + "/store")
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	sig := &Signals{ch: make(chan os.Signal, 1)}
	payload := `{"session_id":"s-label","tool_use_id":"toolu_label","tool_name":"Read","tool_input":{"file_path":"/tmp/x.go"}}`

	h := New(st, now)
	err = h.Capture(bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatal(err)
	}
	h.labelRecovered = true // what fileLabel reports when its recover ran
	h.Close(sig, nil)

	run, err := st.ReadRun("s-label")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 1 || len(run.Terminals) != 1 || run.Terminals[0].Outcome != store.OutcomeOK {
		t.Fatalf("declarations %d, terminals %+v: the call itself must stand", len(run.Declarations), run.Terminals)
	}
	last := run.Coverage[len(run.Coverage)-1]
	if last.Reason == nil || *last.Reason != store.ReasonRecoveredPanic || last.State != store.StateUnverified {
		t.Errorf("coverage = %s %v, want unverified with %s", last.State, last.Reason, store.ReasonRecoveredPanic)
	}
}
