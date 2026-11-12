package main

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestPlanPort_CarriesPathFactsOnly: the port hands the hook the facts that
// name a path a rule covers, planned or held, with the rule id, kind and
// trust word, and nothing for a boundary caution (it names units, not a path).
func TestPlanPort_CarriesPathFactsOnly(t *testing.T) {
	p := newPlanFx(t)
	sp := spec1(spec.Managed, protectGraphDir("p1"), hookioBoundary("internal/graph", "internal/hookio"))
	items, ok := planPort().Check([]byte("change internal/graph/g.go and add internal/graph/brand_new.go"), p.root, sp)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %+v ok=%v, want two path facts", items, ok)
	}
	for _, it := range items {
		if it.RuleID != "p1" || it.Kind != PlanKindProtect || it.Trust != "managed" || it.Path == "" {
			t.Errorf("item = %+v, want a protect_path fact of p1, trust managed", it)
		}
	}
	if items[0].Planned == items[1].Planned {
		t.Errorf("planned flags = %v, %v, want one held and one planned", items[0].Planned, items[1].Planned)
	}
	if got, ok := planPort().Check([]byte("nothing here"), p.root, sp); ok || got != nil {
		t.Errorf("a plan naming nothing: %+v ok=%v, want none", got, ok)
	}
}
