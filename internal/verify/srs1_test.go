package verify

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSRS1e2_TheReplayKeepsABindingViolationRed (card SPEC-RULES-STOP1, #173
// F4): a trusted protect_path rule violated by an edit, then a green
// required gate. The live Stop (no pins) and the replay (the pins laststop
// fills from the checkpoint's gates) both read record_green false: the two
// count the same thing.
func TestSRS1e2_TheReplayKeepsABindingViolationRed(t *testing.T) {
	sp := ruleSpec(spec.Trusted, frozen(allModes...))
	r := runOf(t, edit(1, "db/migrations/001.sql"), gateCall(2, "go-test", store.ExecOK))
	pt := stopPoint(r)
	live := EvaluateStop(r, nil, sp, pt, time.Time{}, StopInput{})
	if v := oneVerdict(t, live.Snap); v.Verdict != Violation {
		t.Fatalf("premise: verdict %+v, want a violation", v)
	}
	pins := map[string]bool{}
	for _, g := range live.Snap.Gates {
		if !g.Required || g.State != Green {
			t.Fatalf("premise: gate %+v, want required and green", g)
		}
		pins[g.Gate] = g.Required
	}
	if len(pins) == 0 {
		t.Fatal("premise: no gate")
	}
	replay := EvaluateStop(r, nil, sp, pt, time.Time{}, StopInput{Required: pins})
	if live.Snap.RecordGreen || replay.Snap.RecordGreen {
		t.Errorf("record_green live %v, replay %v; want both false over a binding violation", live.Snap.RecordGreen, replay.Snap.RecordGreen)
	}
}
