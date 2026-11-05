package hookio_test

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/events"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
)

// TestEvents_CoverHooks (W-M1.7, AD9, AC-X.5): every hook event the
// installer subscribes to maps to a canonical kind, a SessionStart's source
// decides between a session start and a boundary, the subagent events the
// boundary work will subscribe to (W-M4.3) already map, and every canonical
// kind but plan (M2's, from the plan-mode tool call) is produced by some
// hook event, so the adapter and the core vocabulary cover each other.
func TestEvents_CoverHooks(t *testing.T) {
	produced := map[events.Kind]bool{}
	for _, h := range install.Events() {
		k, form, ok := hookio.EventKind(h, "")
		if !ok || !events.Known(k, form) {
			t.Errorf("%s maps to %q/%q, %v; want a known kind", h, k, form, ok)
		}
		produced[k] = true
	}
	for src, want := range map[string][2]string{
		"":        {string(events.SessionStart), ""},
		"startup": {string(events.SessionStart), "startup"},
		"compact": {string(events.Boundary), events.Compact},
		"resume":  {string(events.Boundary), events.Resume},
		"clear":   {string(events.Boundary), events.Clear},
	} {
		k, form, ok := hookio.EventKind(install.EventSessionStart, src)
		if !ok || string(k) != want[0] || form != want[1] {
			t.Errorf("SessionStart source %q = %q/%q, %v; want %q/%q", src, k, form, ok, want[0], want[1])
		}
		produced[k] = true
	}
	for h, want := range map[string]string{"SubagentStart": events.SubagentStart, "SubagentStop": events.SubagentStop} {
		if k, form, ok := hookio.EventKind(h, ""); !ok || k != events.Boundary || form != want {
			t.Errorf("%s = %q/%q, %v; want boundary/%s", h, k, form, ok, want)
		}
	}
	for _, pair := range [][2]string{{install.EventPreToolUse, string(events.ActDeclared)}, {install.EventPostToolUse, string(events.ActLanded)},
		{install.EventPostToolUseFailure, string(events.ActFailed)}, {install.EventUserPromptSubmit, string(events.Prompt)},
		{install.EventStop, string(events.Stop)}, {install.EventStopFailure, string(events.StopFailed)}, {install.EventSessionEnd, string(events.SessionEnd)}} {
		if k, _, _ := hookio.EventKind(pair[0], ""); string(k) != pair[1] {
			t.Errorf("%s = %q; want %s", pair[0], k, pair[1])
		}
	}
	if _, _, ok := hookio.EventKind("Notification", ""); ok {
		t.Errorf("an unsubscribed hook event mapped")
	}
	for _, k := range events.Kinds() {
		if !produced[k] && k != events.Plan {
			t.Errorf("no hook event produces %s", k)
		}
	}
}
