package verify_test

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestIntent_GateIDIsIdFamily: the id intent states for a declared gate is
// the string the Stop's row for that gate carries, the family its rows are
// keyed by (a declared row's Gate, idFamily), one spec read by both: a
// requirement a line names by that id reaches the Stop's gate_green
// unchanged.
func TestIntent_GateIDIsIdFamily(t *testing.T) {
	yes := true
	sp := &spec.Compiled{Sources: []spec.Source{{Path: "/repo/.claude/rashomon/spec.json", Digest: "sha256:0", Trust: spec.Trusted}},
		Trust: spec.Trusted, Gates: []spec.Declared{{ID: "spec:unit",
			Gate: spec.Gate{ID: "unit", Kind: "test", Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: &yes}}}}
	in := intent.Resolve([]byte("fix the parser"), false, intent.Project{Root: "/repo", Spec: sp})
	if len(in.Gates) != 1 {
		t.Fatalf("intent states %d gates, want 1: %+v", len(in.Gates), in.Gates)
	}
	// One verified post coverage record, as runOf adds: a run with none
	// reads incomplete (coverage_absent), and an incomplete record lists no
	// gate row at all (Evaluate returns before the declared gates).
	run := &store.Run{Dir: "s1", Coverage: []store.Coverage{{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		SessionID: "s1", Phase: store.PhasePost, State: store.StateVerified, HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}}}
	v := verify.EvaluateStop(run, nil, sp, verify.Point{Kind: verify.PointStop, SessionID: "s1"}, time.Time{},
		verify.StopInput{Key: []byte("intent-gate-id-test-key")})
	if v.Snap.Recording != verify.Complete {
		t.Fatalf("premise: the record reads %s %v, want complete", v.Snap.Recording, v.Snap.Notes)
	}
	var rows []string
	for _, g := range v.Snap.Gates {
		if g.Family == "declared" {
			rows = append(rows, g.Gate)
		}
	}
	if len(rows) != 1 || in.Gates[0].ID != rows[0] {
		t.Errorf("intent states gate id %q; the Stop's declared rows are %q", in.Gates[0].ID, rows)
	}
}
