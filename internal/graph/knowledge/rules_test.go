package knowledge

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

func declRow(id, status, tier string, preds ...Predicate) Row {
	return Row{Schema: Schema, ID: id, Kind: KindDecision, Title: "Title " + id, Clause: "Keep " + id + " untouched.",
		Status: status, Tier: tier, AppliesTo: []string{"file:go:internal/spec/spec.go"}, Predicates: preds}
}

// parseCompiled marshals rules into a spec.json and runs spec.Parse on it.
func parseCompiled(t *testing.T, rules []spec.Rule) *spec.File {
	t.Helper()
	b, err := json.Marshal(map[string]any{"rashomon_spec": 1, "rules": rules})
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := spec.Parse(b)
	if err != nil {
		t.Fatalf("compiled rules refused by spec.Parse: %v\n%s", err, b)
	}
	return f
}

// TestRules_ApprovedDecisionWithPredicateCompiles: an approved DECLARED
// decision row naming a known predicate compiles to one decision rule that
// spec.Parse accepts, with the row's statement as Why and the fixed sentence
// naming rashomon and the row id as HardControl.
func TestRules_ApprovedDecisionWithPredicateCompiles(t *testing.T) {
	row := declRow("D161", StatusApproved, TierDeclared, Predicate{Kind: spec.PredicatePathsUntouched, Arg: "internal/spec/**"})
	got, refused := Rules([]Row{row})
	if len(refused) != 0 || len(got) != 1 {
		t.Fatalf("got %d rules, refusals %v", len(got), refused)
	}
	r := got[0]
	if r.Kind != string(spec.Decision) || r.Predicate != spec.PredicatePathsUntouched || r.Source != "D161" ||
		r.Why != row.Clause || !strings.Contains(r.HardControl, "rashomon") || !strings.Contains(r.HardControl, "D161") ||
		!slices.Equal(r.AppliesTo, row.AppliesTo) || !slices.Equal(r.Paths, []string{"internal/spec/**"}) {
		t.Errorf("compiled %+v", r)
	}
	f := parseCompiled(t, got)
	if len(f.Rules) != 1 || f.Rules[0].ID != r.ID {
		t.Errorf("parsed back %+v", f.Rules)
	}
	if !slices.Contains(spec.UnenforcedRuleKinds, spec.Decision) {
		t.Error("decision became enforced: the nudge-only statement in the card no longer holds")
	}
}

// TestRules_CandidateRowCompilesToNothing: a candidate, an INFERRED row, a
// retired row, and a row with prose and no predicate each
// compile to nothing, and none is a refusal.
func TestRules_CandidateRowCompilesToNothing(t *testing.T) {
	p := Predicate{Kind: spec.PredicateSymbolPresent, Arg: "Parse"}
	rows := []Row{
		declRow("C1", StatusCandidate, TierDeclared, p),
		declRow("I1", StatusApproved, TierInferred, p),
		declRow("R1", StatusRetired, TierDeclared, p),
		declRow("P1", StatusApproved, TierDeclared),
	}
	got, refused := Rules(rows)
	if len(got) != 0 || len(refused) != 0 {
		t.Errorf("rules %v refusals %v, want none", got, refused)
	}
	if ok, _ := Rules([]Row{declRow("C1", StatusApproved, TierDeclared, p)}); len(ok) != 1 {
		t.Error("control: the same row approved did not compile")
	}
}

// TestRules_UnknownPredicateRefusedAndReported: an unknown predicate is
// returned as a Refusal naming the row and predicate; a known one on the
// same row still compiles.
func TestRules_UnknownPredicateRefusedAndReported(t *testing.T) {
	row := declRow("D9", StatusApproved, TierDeclared,
		Predicate{Kind: "no_such_check", Arg: "x"}, Predicate{Kind: spec.PredicateSymbolPresent, Arg: "Parse"})
	got, refused := Rules([]Row{row})
	if len(got) != 1 || got[0].Predicate != spec.PredicateSymbolPresent {
		t.Errorf("rules %+v", got)
	}
	if len(refused) != 1 || refused[0].Row != "D9" || refused[0].Predicate != "no_such_check" || refused[0].Reason == "" {
		t.Errorf("refusals %+v", refused)
	}
	parseCompiled(t, got)
}

// TestRules_Deterministic: the same rows give the same rules and refusals,
// and the ids of two predicates on one row differ.
func TestRules_Deterministic(t *testing.T) {
	rows := []Row{
		declRow("B2", StatusApproved, TierDeclared, Predicate{Kind: spec.PredicatePathsUntouched, Arg: "a/**"}, Predicate{Kind: spec.PredicateSymbolPresent, Arg: "F"}),
		declRow("A1", StatusApproved, TierDeclared, Predicate{Kind: "zzz"}, Predicate{Kind: spec.PredicateSymbolPresent, Arg: "G"}),
	}
	a1, r1 := Rules(rows)
	for range 20 {
		a2, r2 := Rules(rows)
		ja, _ := json.Marshal(a1)
		jb, _ := json.Marshal(a2)
		if string(ja) != string(jb) || !slices.Equal(r1, r2) {
			t.Fatal("output changed between calls")
		}
	}
	if len(a1) != 3 || a1[0].Source != "B2" || a1[2].Source != "A1" {
		t.Errorf("not in input order: %+v", a1)
	}
	if a1[0].ID == a1[1].ID {
		t.Errorf("two rules share id %q", a1[0].ID)
	}
	parseCompiled(t, a1)
}

// TestRules_RowKindSwitch: E2's one switch. Decision rows compile; an
// approved DECLARED row of another kind with a known predicate is one
// Refusal "row kind not compiled" and no rule, so an approved check the
// binary ignores is countable. Flipping rowKindCompiles is the whole ruling.
func TestRules_RowKindSwitch(t *testing.T) {
	p := Predicate{Kind: spec.PredicatePathsUntouched, Arg: "a/**"}
	req := declRow("PF-1", StatusApproved, TierDeclared, p)
	req.Kind = KindRequirement
	if rowKindCompiles(KindRequirement) || !rowKindCompiles(KindDecision) {
		t.Error("rowKindCompiles must admit decision rows only")
	}
	got, refused := Rules([]Row{declRow("D1", StatusApproved, TierDeclared, p), req})
	if len(got) != 1 || got[0].Source != "D1" {
		t.Errorf("rules %+v, want the decision row's only", got)
	}
	if len(refused) != 1 || refused[0].Row != "PF-1" || refused[0].Predicate != spec.PredicatePathsUntouched ||
		refused[0].Reason != "row kind not compiled" {
		t.Errorf("refusals %+v, want one 'row kind not compiled' for PF-1", refused)
	}
	// Not admitted at all stays silent: another kind that is a candidate, or has an unknown predicate only.
	cand := declRow("PF-2", StatusCandidate, TierDeclared, p)
	cand.Kind = KindRequirement
	unk := declRow("PF-3", StatusApproved, TierDeclared, Predicate{Kind: "nope"})
	unk.Kind = KindRequirement
	if g, r := Rules([]Row{cand, unk}); len(g) != 0 || len(r) != 0 {
		t.Errorf("rules %v refusals %v, want none", g, r)
	}
}

// TestRules_IDAndTextBounds: a row id that is not a valid spec id and a
// clause over the limit still parse.
func TestRules_IDAndTextBounds(t *testing.T) {
	row := declRow("ADR-0007 Some/Long "+strings.Repeat("x", 80), StatusApproved, TierDeclared, Predicate{Kind: spec.PredicateSymbolPresent, Arg: "F"})
	row.Clause = strings.Repeat("é", 400) + "\n"
	got, _ := Rules([]Row{row})
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	parseCompiled(t, got)
	empty := declRow("E1", StatusApproved, TierDeclared, Predicate{Kind: spec.PredicateSymbolPresent})
	empty.Clause, empty.Title = "", ""
	if g, refused := Rules([]Row{empty}); len(g) != 0 || len(refused) != 1 {
		t.Errorf("statement-less row: rules %v refusals %v", g, refused)
	}
}
