package engine

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

const rulesRID = "github.com/example/rules"

// rulesSpec is a compiled spec with one source of the given trust.
func rulesSpec(trust spec.Trust, rules ...spec.Rule) *spec.Compiled {
	sp := &spec.Compiled{Sources: []spec.Source{{Path: "spec.json", Trust: trust}}}
	for _, r := range rules {
		sp.Rules = append(sp.Rules, spec.DeclaredRule{ID: r.ID, Source: 0, Rule: r})
	}
	return sp
}

func rulesRow(status string, applies ...graph.ID) knowledge.Row {
	var ids []string
	for _, a := range applies {
		ids = append(ids, string(a))
	}
	return knowledge.Row{Schema: 1, ID: "D7", Kind: knowledge.KindDecision, Title: "t", Clause: "keep it",
		Status: status, Tier: knowledge.TierDeclared, AppliesTo: ids,
		Predicates: []knowledge.Predicate{{Kind: spec.PredicatePathsUntouched}}}
}

// TestRules_ApprovedDecisionRowReachesConflicts: an approved DECLARED
// decision row becomes a Rule whose AppliesTo is the local id at rev, and
// Conflicts raises on it. Controls: the same row as a candidate gives none;
// an id of another repository is dropped, never matched by name; an id the
// index lacks at rev is dropped.
func TestRules_ApprovedDecisionRowReachesConflicts(t *testing.T) {
	f := newFx()
	file := f.file("hookio", "hookio/h.lx", fxC1)
	d := Diff{Rev: fxC1, Files: []graph.ID{file}}

	rules := RulesFor(nil, []knowledge.Row{rulesRow(knowledge.StatusApproved, graph.GlobalID(rulesRID, file))}, f.s, rulesRID, fxC1)
	if len(rules) != 1 || rules[0].Kind != "decision" || rules[0].Predicate != spec.PredicatePathsUntouched ||
		len(rules[0].AppliesTo) != 1 || rules[0].AppliesTo[0] != file {
		t.Fatalf("rules = %+v, want one decision on %s", rules, file)
	}
	if got := Conflicts(f.s, d, rules); len(got) != 1 || got[0].Kind != ConflictDecisionContradicted {
		t.Errorf("conflicts = %s, want one decision_contradicted", cstr(got))
	}

	cand := RulesFor(nil, []knowledge.Row{rulesRow(knowledge.StatusCandidate, graph.GlobalID(rulesRID, file))}, f.s, rulesRID, fxC1)
	if got := Conflicts(f.s, d, cand); len(cand) != 0 || len(got) != 0 {
		t.Errorf("candidate row: rules %+v conflicts %s, want none", cand, cstr(got))
	}
	foreign := RulesFor(nil, []knowledge.Row{rulesRow(knowledge.StatusApproved, graph.GlobalID("github.com/other/repo", file))}, f.s, rulesRID, fxC1)
	if got := Conflicts(f.s, d, foreign); len(got) != 0 {
		t.Errorf("foreign repo id matched by name: %s", cstr(got))
	}
	absent := RulesFor(nil, []knowledge.Row{rulesRow(knowledge.StatusApproved, graph.GlobalID(rulesRID, "file:nope.lx"))}, f.s, rulesRID, fxC1)
	if len(absent) != 1 || len(absent[0].AppliesTo) != 0 {
		t.Errorf("absent id kept: %+v", absent)
	}
}

// TestRules_SpecProtectPathReachesConflicts: a trusted spec's protect_path
// rule carries its Paths into the rule and fires on a planned new file under
// the pattern; an untrusted source's rule is withheld, and so is a rule
// whose source is out of range.
func TestRules_SpecProtectPathReachesConflicts(t *testing.T) {
	f := newFx()
	rule := spec.Rule{ID: "p1", Kind: string(spec.ProtectPath), Why: "w", HardControl: "h", Paths: []string{"secrets"}}
	d := Diff{Rev: fxC1, Files: []graph.ID{graph.FileID("secrets/new.txt")}}

	rules := RulesFor(rulesSpec(spec.Trusted, rule), nil, f.s, rulesRID, fxC1)
	if got := Conflicts(f.s, d, rules); len(got) != 1 || got[0].Kind != ConflictProtectedPath {
		t.Errorf("trusted: rules %+v conflicts %s, want one protected_path", rules, cstr(got))
	}
	if rules := RulesFor(rulesSpec(spec.Untrusted, rule), nil, f.s, rulesRID, fxC1); len(rules) != 0 {
		t.Errorf("untrusted: %+v, want withheld", rules)
	}
	stray := rulesSpec(spec.Managed, rule)
	stray.Rules[0].Source = 5
	if rules := RulesFor(stray, nil, f.s, rulesRID, fxC1); len(rules) != 0 {
		t.Errorf("source out of range: %+v, want withheld", rules)
	}
}

// TestRules_ManagedBoundaryRuleReachesCaution: a boundary rule whose source
// is Managed reaches the plan-time caution; the same rule from an Untrusted
// source gives none.
func TestRules_ManagedBoundaryRuleReachesCaution(t *testing.T) {
	f := newFx()
	x, y := fxUnit("a"), fxUnit("b")
	rule := spec.Rule{ID: "b1", Kind: string(spec.Boundary), Why: "w", HardControl: "h", From: []string{string(x)}, To: []string{string(y)}}

	rules := RulesFor(rulesSpec(spec.Managed, rule), nil, f.s, rulesRID, fxC1)
	got := BoundaryCautions([]graph.ID{x, y}, rules)
	if len(got) != 1 || got[0] != (Caution{RuleID: "b1", From: x, To: y}) {
		t.Errorf("managed: %+v, want one b1 a into b", got)
	}
	rules = RulesFor(rulesSpec(spec.Untrusted, rule), nil, f.s, rulesRID, fxC1)
	if got := BoundaryCautions([]graph.ID{x, y}, rules); len(got) != 0 {
		t.Errorf("untrusted: %+v, want none", got)
	}
}

// TestRules_GlobalAppliesToFiresPathsUntouched (R9): a decision row whose
// applies_to is in the documented global form for this repository fires
// paths_untouched on a diff touching its path, through RulesFor's
// localisation against the header's repository.
func TestRules_GlobalAppliesToFiresPathsUntouched(t *testing.T) {
	f := newFx()
	file := f.file("hookio", "hookio/h.lx", fxC1)
	rows := []knowledge.Row{rulesRow(knowledge.StatusApproved, graph.GlobalID(rulesRID, file))}
	rules := RulesFor(nil, rows, f.s, rulesRID, fxC1)
	got := onlyKind(Conflicts(f.s, Diff{Rev: fxC1, Files: []graph.ID{file}}, rules), ConflictDecisionContradicted)
	if len(got) != 1 || got[0].Subject != file {
		t.Errorf("global form for this repo: %s, want one on %s", cstr(got), file)
	}
}

// TestRules_ForeignRepoAppliesToDoesNotFire (R9): a global id of another
// repository is dropped, never matched by its local part; a local-form id
// passes through unchanged and fires.
func TestRules_ForeignRepoAppliesToDoesNotFire(t *testing.T) {
	f := newFx()
	file := f.file("hookio", "hookio/h.lx", fxC1)
	d := Diff{Rev: fxC1, Files: []graph.ID{file}}
	foreign := RulesFor(nil, []knowledge.Row{rulesRow(knowledge.StatusApproved, graph.GlobalID("github.com/other/repo", file))}, f.s, rulesRID, fxC1)
	if got := Conflicts(f.s, d, foreign); len(got) != 0 || len(foreign) != 1 || len(foreign[0].AppliesTo) != 0 {
		t.Errorf("foreign repo: rules %+v conflicts %s, want the rule with no target and none raised", foreign, cstr(got))
	}
	local := RulesFor(nil, []knowledge.Row{rulesRow(knowledge.StatusApproved, file)}, f.s, rulesRID, fxC1)
	if got := Conflicts(f.s, d, local); len(got) != 1 {
		t.Errorf("local form: %s, want one (passes through unchanged)", cstr(got))
	}
}

// TestRules_RowKeepsItsAppliesToEdgeInRecords (R9): localising never
// mutates the row; its knowledge Records still carry the applies_to edge.
func TestRules_RowKeepsItsAppliesToEdgeInRecords(t *testing.T) {
	f := newFx()
	file := f.file("hookio", "hookio/h.lx", fxC1)
	global := graph.GlobalID(rulesRID, file)
	rows := []knowledge.Row{rulesRow(knowledge.StatusApproved, global)}
	_ = RulesFor(nil, rows, f.s, rulesRID, fxC1)
	if len(rows[0].AppliesTo) != 1 || rows[0].AppliesTo[0] != string(global) {
		t.Fatalf("row AppliesTo = %v, want [%s]", rows[0].AppliesTo, global)
	}
	var edge *graph.Edge
	for _, r := range knowledge.Records(rows, rulesRID, fxC1) {
		if r.Edge != nil && r.Edge.Pred == graph.AppliesTo {
			edge = r.Edge
		}
	}
	if edge == nil || edge.Dst != file {
		t.Errorf("applies_to edge = %+v, want one to %s", edge, file)
	}
}
