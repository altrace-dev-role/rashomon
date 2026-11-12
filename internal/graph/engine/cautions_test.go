package engine

import (
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestPlan_BoundaryCautionWhenPlanNamesBothSides: a caution per boundary
// rule when the plan's units hold one under From and one under To. Controls:
// only one side named gives none; a rule From Y To X gives exactly one, in
// that direction; a rule of another kind gives none.
func TestPlan_BoundaryCautionWhenPlanNamesBothSides(t *testing.T) {
	x, y := fxUnit("internal/graph/engine"), fxUnit("internal/hookio")
	rule := boundaryRule("b1", graphU, hookU)

	got := BoundaryCautions([]graph.ID{y, x}, []Rule{rule})
	want := []Caution{{RuleID: "b1", From: x, To: y}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("both sides named: %+v, want %+v", got, want)
	}
	if got := BoundaryCautions([]graph.ID{x}, []Rule{rule}); len(got) != 0 {
		t.Errorf("only the From side: %+v, want none", got)
	}
	if got := BoundaryCautions([]graph.ID{y}, []Rule{rule}); len(got) != 0 {
		t.Errorf("only the To side: %+v, want none", got)
	}
	rev := boundaryRule("b2", hookU, graphU)
	got = BoundaryCautions([]graph.ID{x, y}, []Rule{rev})
	want = []Caution{{RuleID: "b2", From: y, To: x}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a rule From hookio To graph: %+v, want %+v", got, want)
	}
	other := Rule{ID: "d1", Kind: "decision", From: []string{graphU}, To: []string{hookU}}
	if got := BoundaryCautions([]graph.ID{x, y}, []Rule{other}); len(got) != 0 {
		t.Errorf("a decision rule: %+v, want none", got)
	}
}
