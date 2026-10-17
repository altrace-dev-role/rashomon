package supervise

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/internal/install"
)

// TestRES26_SoftBudgetNeverExceedsTheRealHardStop checks every
// DeadlineEntry's table soft deadline against the real hard stop a
// supervised entry gets for its own event(s): install.TimeoutFor's
// per-event timeout, through the same HardStop this package's Child
// shares with its parent for the kill. A soft deadline at or past that
// hard stop would let an optional phase still be running after the entry
// has already been killed, which is the gap build spec 8.2.5 exists to
// close.
//
// This replaces a version of this test that rebuilt the hard-stop formula
// from installedS/shortSession literals invented per case rather than
// going through HardStop and a real event name, and so drifted from the
// production margin it meant to pin (EntryProbeEnd's real hard stop is
// 1.4s; the old cases checked it against an invented 1.9s).
func TestRES26_SoftBudgetNeverExceedsTheRealHardStop(t *testing.T) {
	cases := []struct {
		e  budget.DeadlineEntry
		ev string
	}{
		{budget.EntryHook, install.EventPreToolUse},
		{budget.EntryPost, install.EventPostToolUse},
		// Neither future `event` row has a production caller yet (P-16),
		// so neither has a real install.Event constant; these are the
		// hook_event_name values the unified command will dispatch on.
		{budget.EntryEventPostToolBatch, "PostToolBatch"},
		{budget.EntryEventOther, "SubagentStart"},
		{budget.EntryRecapPending, install.EventUserPromptSubmit},
		{budget.EntryRecapStop, install.EventStop},
		{budget.EntryProbeStart, install.EventSessionStart},
		{budget.EntryProbeEnd, install.EventSessionEnd},
	}
	for _, c := range cases {
		hard := HardStop(install.TimeoutFor(c.ev), []string{c.ev}, "")
		if soft := c.e.SoftBudget(); soft >= hard {
			t.Errorf("%v (%s): soft budget %v leaves no room before the real hard stop %v", c.e, c.ev, soft, hard)
		}
	}
}
