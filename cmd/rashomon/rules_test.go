package main

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

func decisionRow(p planFx, status string, applies ...graph.ID) knowledge.Row {
	return knowledge.Row{Schema: 1, ID: "D7", Kind: knowledge.KindDecision, Title: "t", Clause: "keep it",
		Status: status, Tier: knowledge.TierDeclared, AppliesTo: func() []string {
			var out []string
			for _, a := range applies {
				out = append(out, string(a))
			}
			return out
		}(), Predicates: []knowledge.Predicate{{Kind: spec.PredicatePathsUntouched}}}
}

// TestRules_ApprovedDecisionRowReachesConflicts: an approved DECLARED
// decision row becomes an engine.Rule whose AppliesTo is the local id at the
// index rev, and Conflicts raises on it. Controls: the same row as a
// candidate gives none; an id of another repository is dropped, never matched
// by name; an id the index lacks at rev is dropped.
func TestRules_ApprovedDecisionRowReachesConflicts(t *testing.T) {
	p := newPlanFx(t)
	file := graph.FileID("internal/hookio/h.go")
	d := engine.Diff{Rev: p.rev, Files: []graph.ID{file}}

	rules := PlanRules(nil, []knowledge.Row{decisionRow(p, knowledge.StatusApproved, graph.GlobalID(p.rid, file))}, p.s, p.rid, p.rev)
	if len(rules) != 1 || rules[0].Kind != "decision" || rules[0].Predicate != spec.PredicatePathsUntouched || len(rules[0].AppliesTo) != 1 || rules[0].AppliesTo[0] != file {
		t.Fatalf("rules = %+v, want one decision on %s", rules, file)
	}
	if got := engine.Conflicts(p.s, d, rules); len(got) != 1 || got[0].Kind != engine.ConflictDecisionContradicted {
		t.Errorf("conflicts = %+v, want one decision_contradicted", got)
	}

	cand := PlanRules(nil, []knowledge.Row{decisionRow(p, knowledge.StatusCandidate, graph.GlobalID(p.rid, file))}, p.s, p.rid, p.rev)
	if got := engine.Conflicts(p.s, d, cand); len(cand) != 0 || len(got) != 0 {
		t.Errorf("candidate row: rules %+v conflicts %+v, want none", cand, got)
	}
	foreign := PlanRules(nil, []knowledge.Row{decisionRow(p, knowledge.StatusApproved, graph.GlobalID("github.com/other/repo", file))}, p.s, p.rid, p.rev)
	if got := engine.Conflicts(p.s, d, foreign); len(got) != 0 {
		t.Errorf("foreign repo id matched by name: %+v", got)
	}
	absent := PlanRules(nil, []knowledge.Row{decisionRow(p, knowledge.StatusApproved, graph.GlobalID(p.rid, "file:nope.go"))}, p.s, p.rid, p.rev)
	if len(absent) != 1 || len(absent[0].AppliesTo) != 0 {
		t.Errorf("absent id kept: %+v", absent)
	}
}

// TestRules_SpecProtectPathReachesConflicts: a trusted spec's protect_path
// rule carries its Paths into the engine rule and fires on a planned new
// file under the pattern; an untrusted source's rule is withheld.
func TestRules_SpecProtectPathReachesConflicts(t *testing.T) {
	p := newPlanFx(t)
	rule := spec.Rule{ID: "p1", Kind: string(spec.ProtectPath), Why: "w", HardControl: "h", Paths: []string{"secrets"}}
	d := engine.Diff{Rev: p.rev, Files: []graph.ID{graph.FileID("secrets/new.txt")}}

	rules := PlanRules(spec1(spec.Trusted, rule), nil, p.s, p.rid, p.rev)
	if got := engine.Conflicts(p.s, d, rules); len(got) != 1 || got[0].Kind != engine.ConflictProtectedPath {
		t.Errorf("trusted: rules %+v conflicts %+v, want one protected_path", rules, got)
	}
	if rules := PlanRules(spec1(spec.Untrusted, rule), nil, p.s, p.rid, p.rev); len(rules) != 0 {
		t.Errorf("untrusted: %+v, want withheld", rules)
	}
}

// TestRules_ManagedBoundaryRuleReachesCaution: a boundary rule whose source
// is Managed reaches the plan-time caution; the same rule from an Untrusted
// source gives none.
func TestRules_ManagedBoundaryRuleReachesCaution(t *testing.T) {
	p := newPlanFx(t)
	touched := []graph.ID{"unit:go:internal/graph", "unit:go:internal/hookio"}
	rule := hookioBoundary("internal/graph", "internal/hookio")

	rules := PlanRules(spec1(spec.Managed, rule), nil, p.s, p.rid, p.rev)
	got := engine.BoundaryCautions(touched, rules)
	if len(got) != 1 || got[0].RuleID != "b1" || got[0].From != touched[0] || got[0].To != touched[1] {
		t.Errorf("managed: %+v, want one b1 graph into hookio", got)
	}
	rules = PlanRules(spec1(spec.Untrusted, rule), nil, p.s, p.rid, p.rev)
	if got := engine.BoundaryCautions(touched, rules); len(got) != 0 {
		t.Errorf("untrusted: %+v, want none", got)
	}
}
