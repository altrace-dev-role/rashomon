package hook

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestMA1h_HiddenRunIsTheAtTheActHint (card MASKED-AT-ACT1): the post reads
// the run for masked-status only on a runner call that ended ok in the
// foreground with its status hidden by its line, a test or a build; verify
// decides from there. A failed line (its failure event steps already), a
// backgrounded launch, a line that names no runner and a runner with its
// status in view are no hint.
func TestMA1h_HiddenRunIsTheAtTheActHint(t *testing.T) {
	digest, test, build, none := "d", shape.MaskedTest, shape.MaskedBuild, shape.MaskedNone
	hidden := func() store.Execution {
		return store.Execution{Outcome: store.ExecOK, RunnerDigest: &digest, StatusMasked: &build}
	}
	for _, tc := range []struct {
		name string
		edit func(e *store.Execution)
		want bool
	}{
		{"a build hidden by its line", func(*store.Execution) {}, true},
		{"a test hidden by its line", func(e *store.Execution) { e.StatusMasked = &test }, true},
		{"a failed line", func(e *store.Execution) { e.Outcome = store.ExecFailed }, false},
		{"a backgrounded launch", func(e *store.Execution) { e.Backgrounded = true }, false},
		{"no runner", func(e *store.Execution) { e.RunnerDigest = nil }, false},
		{"the status in view", func(e *store.Execution) { e.StatusMasked = &none }, false},
		{"no masking read", func(e *store.Execution) { e.StatusMasked = nil }, false},
	} {
		e := hidden()
		tc.edit(&e)
		if got := hiddenRun(&e); got != tc.want {
			t.Errorf("%s: hiddenRun = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestMA1p_AnUnsummedPipedRunSaysItsStatusIsHidden (card MASKED-AT-ACT1): a
// piped pytest after a green and a write, whose output holds no summary or
// nothing, steps at its own PostToolUse and says only masked-status: the
// tail hid the exit and the output did not say how the run ended. No
// regression-location, which needs a red.
func TestMA1p_AnUnsummedPipedRunSaysItsStatusIsHidden(t *testing.T) {
	const note = "gate 'pytest' ran in this call and the pipe to tail hid its exit status; no outcome of this run is recorded."
	for _, tc := range []struct{ name, stdout string }{
		{"no summary", "collecting ...\n"},
		{"empty stdout", ""},
	} {
		out := string(pipedAfterGreen(t, tc.stdout))
		if !strings.Contains(out, note) {
			t.Errorf("%s: printed %q, want the note %q", tc.name, out, note)
		}
		if strings.Contains(out, "passed earlier") || strings.Count(out, "rashomon record:") != strings.Count(out, note) {
			t.Errorf("%s: printed %q, want masked-status alone", tc.name, out)
		}
	}
}
