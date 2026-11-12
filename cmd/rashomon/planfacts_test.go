package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

func protectGraphDir(id string) spec.Rule {
	return spec.Rule{ID: id, Kind: string(spec.ProtectPath), Why: "w", HardControl: "h", Paths: []string{"internal/graph/*.go"}}
}

// TestPlanFacts_PlannedFileReachesPathRulesOnly (finding D, pins 1 and 5): a
// file the plan names that is not on disk, under a directory that is, falls
// under a path rule matching its path, once; a missing parent gives nothing;
// a boundary rule never reads it; the fact says planned and carries the rule
// and its kind.
func TestPlanFacts_PlannedFileReachesPathRulesOnly(t *testing.T) {
	p := newPlanFx(t)
	sp := spec1(spec.Managed, protectGraphDir("p1"), hookioBoundary("internal/graph", "internal/hookio"))

	facts, ok := PlanFacts([]byte("add internal/graph/brand_new.go"), p.root, sp)
	if !ok || len(facts) != 1 {
		t.Fatalf("facts = %+v ok=%v, want one", facts, ok)
	}
	f := facts[0]
	if !f.Planned || f.Path != "internal/graph/brand_new.go" || f.RuleID != "p1" || f.Kind != PlanKindProtect || f.Trust != "managed" {
		t.Errorf("fact = %+v, want a planned protect_path fact of p1 on the new file, trust managed", f)
	}
	// PlanLines prints no line for a planned file.
	if lines, ok := PlanLines([]byte("add internal/graph/brand_new.go"), p.root, sp); ok {
		t.Errorf("PlanLines = %q, want none for a planned file", lines)
	}
	// A missing parent gives nothing.
	if facts, ok := PlanFacts([]byte("add internal/nodir/brand_new.go"), p.root, sp); ok {
		t.Errorf("missing parent: %+v, want none", facts)
	}
	// A path outside the pattern gives nothing.
	if facts, ok := PlanFacts([]byte("add internal/hookio/brand_new.go"), p.root, sp); ok {
		t.Errorf("outside the pattern: %+v, want none", facts)
	}
}

// TestPlanFacts_PlannedAndExistingUnderOnePattern (pin 6): both are facts,
// one each, and PlanLines prints the existing one alone.
func TestPlanFacts_PlannedAndExistingUnderOnePattern(t *testing.T) {
	p := newPlanFx(t)
	sp := spec1(spec.Managed, protectGraphDir("p1"))
	plan := []byte("change internal/graph/g.go and add internal/graph/brand_new.go")
	facts, ok := PlanFacts(plan, p.root, sp)
	if !ok || len(facts) != 2 {
		t.Fatalf("facts = %+v ok=%v, want two", facts, ok)
	}
	var planned, held []string
	for _, f := range facts {
		if f.Planned {
			planned = append(planned, f.Path)
		} else {
			held = append(held, f.Path)
		}
	}
	if !slices.Equal(planned, []string{"internal/graph/brand_new.go"}) || !slices.Equal(held, []string{"internal/graph/g.go"}) {
		t.Errorf("planned %v, held %v", planned, held)
	}
	lines, ok := PlanLines(plan, p.root, sp)
	if !ok || len(lines) != 1 || !strings.Contains(lines[0], "internal/graph/g.go") || strings.Contains(lines[0], "brand_new") {
		t.Errorf("PlanLines = %q ok=%v, want the one line on the existing file", lines, ok)
	}
}

// TestPlanFacts_DecisionRowStaysUntouched (A3): an existing path keeps the
// decision row's paths_untouched branch, worded as before and as the fact's
// kind.
func TestPlanFacts_DecisionRowStaysUntouched(t *testing.T) {
	p := newPlanFx(t)
	row := knowledge.Row{Schema: 1, ID: "D1", Kind: knowledge.KindDecision, Title: "leave hookio", Clause: "hookio stays as is",
		Status: knowledge.StatusApproved, Tier: knowledge.TierDeclared,
		AppliesTo:  []string{string(graph.GlobalID(p.rid, "file:internal/hookio/h.go"))},
		Predicates: []knowledge.Predicate{{Kind: spec.PredicatePathsUntouched}}}
	if _, err := knowledge.WriteFile(p.root, "d", []knowledge.Row{row}); err != nil {
		t.Fatal(err)
	}
	plan := []byte("change internal/hookio/h.go")
	facts, ok := PlanFacts(plan, p.root, spec1(spec.Managed))
	if !ok || len(facts) != 1 || facts[0].Kind != PlanKindUntouched || facts[0].RuleID == "" || facts[0].Path != "internal/hookio/h.go" || facts[0].Planned {
		t.Fatalf("facts = %+v ok=%v, want one paths_untouched fact on h.go", facts, ok)
	}
	lines, _ := PlanLines(plan, p.root, spec1(spec.Managed))
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "the plan names internal/hookio/h.go; decision ") || !strings.HasSuffix(lines[0], " says it stays untouched") ||
		!strings.Contains(lines[0], "(D1)") {
		t.Errorf("lines = %q, want the stays-untouched line from row D1", lines)
	}
}
