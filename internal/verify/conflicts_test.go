package verify

import (
	"encoding/json"
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

func seen(kind, rule, unit string, resolved int64, seqs ...int64) ConflictSeen {
	return ConflictSeen{Row: ConflictRow{Kind: kind, Unit: unit, UnitPath: strings.TrimPrefix(unit, "g:r#unit:go:"), RuleID: rule,
		Subject: "sym:go:" + unit}, Seqs: seqs, ResolvedSeq: resolved}
}

const unitA, unitB = "g:r#unit:go:internal/a", "g:r#unit:go:internal/b"

// TestObligations_ConflictRowCarriesUnitAndRule (PLAN): one unresolved
// conflict is one spec_rule row in the session scope, sourced by the rule
// id, carrying the unit, unmet, citing the seqs that named it.
func TestObligations_ConflictRowCarriesUnitAndRule(t *testing.T) {
	got := conflictObligations(ScopeSession, []ConflictSeen{seen("boundary_cross", "no-ui-db", unitA, 0, 3, 5)})
	if len(got) != 1 {
		t.Fatalf("rows %+v, want one", got)
	}
	o := got[0]
	if o.Kind != SpecRule || o.Source != "no-ui-db" || o.Unit != unitA || o.Scope != ScopeSession ||
		o.State != ObligationUnmet || !slices.Equal(o.Evidence, []int64{3, 5}) {
		t.Errorf("row %+v, want spec_rule, source no-ui-db, unit %s, session, unmet, evidence [3 5]", o, unitA)
	}
}

// TestObligations_ConflictIDNeverEqualsVerdictID (B4): a conflict row never
// takes the id of the verdict row for the same rule, whichever form the rule
// id has.
func TestObligations_ConflictIDNeverEqualsVerdictID(t *testing.T) {
	for _, rule := range []string{"no-ui-db", "spec:no-ui-db", "k:decision-7", "no ui -> db: bad", "c8:boundary_cross"} {
		verdict := ObligationID(ScopeSession, SpecRule, rule)
		for _, kind := range ConflictRowKinds {
			for _, unit := range []string{unitA, "", rule, "spec:" + rule} {
				fam := conflictFamily(ConflictRow{Kind: kind, RuleID: rule, Unit: unit})
				if id := ObligationID(ScopeSession, SpecRule, fam); id == verdict {
					t.Errorf("rule %q kind %s unit %q: conflict id %s equals the verdict id", rule, kind, unit, id)
				}
				if fam == rule || fam == "spec:"+rule {
					t.Errorf("rule %q: family %q equals a verdict family", rule, fam)
				}
			}
		}
	}
}

// TestObligations_ConflictIDInjective (B4): distinct per unit, and no pair of
// (rule, unit) splits the same string two ways.
func TestObligations_ConflictIDInjective(t *testing.T) {
	ids := map[string]string{}
	for _, kind := range ConflictRowKinds {
		for _, rule := range []string{"", "a", "ab", "a:b", "1:a", "a b"} {
			for _, unit := range []string{"b", "bc", "c", "1:ab", "g:r#unit:go:x"} {
				id := ObligationID(ScopeSession, SpecRule, conflictFamily(ConflictRow{Kind: kind, RuleID: rule, Unit: unit}))
				key := kind + "|" + rule + "|" + unit
				if prev, dup := ids[id]; dup {
					t.Errorf("%s and %s share id %s", prev, key, id)
				}
				ids[id] = key
			}
		}
	}
	a := conflictObligations(ScopeSession, []ConflictSeen{seen("boundary_cross", "a", "bc", 0, 1), seen("boundary_cross", "ab", "c", 0, 1)})
	if len(a) != 2 || a[0].ID == a[1].ID {
		t.Errorf("rule a unit bc vs rule ab unit c: %+v, want two distinct ids", a)
	}
}

// TestObligations_SameRuleTwoUnitsTwoRows (B4).
func TestObligations_SameRuleTwoUnitsTwoRows(t *testing.T) {
	got := conflictObligations(ScopeSession, []ConflictSeen{seen("boundary_cross", "r", unitB, 0, 4), seen("boundary_cross", "r", unitA, 0, 3)})
	if len(got) != 2 || got[0].Unit != unitA || got[1].Unit != unitB || got[0].ID == got[1].ID {
		t.Errorf("rows %+v, want two, units a then b, distinct ids", got)
	}
}

// TestObligations_OneOfTwoCrossesRemovedStaysUnmet (B4): two crosses of one
// rule in one unit are one row; one removed leaves it unmet, citing both.
func TestObligations_OneOfTwoCrossesRemovedStaysUnmet(t *testing.T) {
	got := conflictObligations(ScopeSession, []ConflictSeen{seen("boundary_cross", "r", unitA, 9, 3), seen("boundary_cross", "r", unitA, 0, 5, 3)})
	if len(got) != 1 || got[0].State != ObligationUnmet || !slices.Equal(got[0].Evidence, []int64{3, 5}) {
		t.Errorf("rows %+v, want one unmet row citing [3 5]", got)
	}
}

// TestObligations_RuleLessKindsInOneUnitAreTwoRows: signature_mismatch and
// protected_path both carry RuleID "" here; only the kind tells them apart.
func TestObligations_RuleLessKindsInOneUnitAreTwoRows(t *testing.T) {
	got := conflictObligations(ScopeSession, []ConflictSeen{seen("protected_path", "", unitA, 0, 2), seen("signature_mismatch", "", unitA, 0, 1)})
	if len(got) != 2 || got[0].ID == got[1].ID {
		t.Fatalf("rows %+v, want two with distinct ids", got)
	}
	if got[0].Source != "conflict:signature_mismatch" || got[1].Source != "conflict:protected_path" {
		t.Errorf("sources %q and %q, want conflict:<kind> in the kinds' order", got[0].Source, got[1].Source)
	}
}

// TestObligations_NewlineYieldsNoRow: a '\n' in Kind, RuleID or Unit, and an
// empty Unit, owe no row.
func TestObligations_NewlineYieldsNoRow(t *testing.T) {
	for name, c := range map[string]ConflictSeen{
		"kind":       seen("boundary_cross\n", "r", unitA, 0, 1),
		"rule":       seen("boundary_cross", "r\nx", unitA, 0, 1),
		"unit":       seen("boundary_cross", "r", "g:r#unit:go:a\nb", 0, 1),
		"empty unit": seen("boundary_cross", "r", "", 0, 1),
	} {
		if got := conflictObligations(ScopeSession, []ConflictSeen{c}); len(got) != 0 {
			t.Errorf("%s: rows %+v, want none", name, got)
		}
	}
}

// TestStop_WithheldConflictKindLeavesNoLedgerRow (ruling 16:23): the ledger and
// the view half. A duplicate_abstraction or arity member alone, and beside a
// live one, yields no Obligation and no StopView.Conflicts entry. The text
// half is cmd/rashomon's.
func TestStop_WithheldConflictKindLeavesNoLedgerRow(t *testing.T) {
	live := seen("boundary_cross", "r", unitA, 0, 3)
	for _, kind := range []string{"duplicate_abstraction", "arity", "no_such_kind"} {
		other := seen(kind, "r", unitB, 0, 4)
		v := stopOf(&store.Run{}, nil, "s1", StopInput{Conflicts: []ConflictSeen{other}})
		if len(v.Conflicts) != 0 || slices.ContainsFunc(v.Obligations, func(o Obligation) bool { return o.Unit != "" }) {
			t.Errorf("%s alone: conflicts %+v obligations %+v, want none", kind, v.Conflicts, v.Obligations)
		}
		v = stopOf(&store.Run{}, nil, "s1", StopInput{Conflicts: []ConflictSeen{other, live}})
		if len(v.Conflicts) != 1 || v.Conflicts[0].Kind != "boundary_cross" || len(v.Obligations) != 1 || v.Obligations[0].Unit != unitA {
			t.Errorf("%s beside a live one: conflicts %+v obligations %+v, want the live one only", kind, v.Conflicts, v.Obligations)
		}
	}
}

// TestStop_ResolvedConflictIsMetWithEvidence (PLAN), the verify half: every
// member resolved is met, citing the edit that removed the last, and the
// view holds no entry for it.
func TestStop_ResolvedConflictIsMetWithEvidence(t *testing.T) {
	v := stopOf(&store.Run{}, nil, "s1", StopInput{Conflicts: []ConflictSeen{
		seen("boundary_cross", "r", unitA, 7, 3), seen("boundary_cross", "r", unitA, 11, 5)}})
	if len(v.Obligations) != 1 || v.Obligations[0].State != ObligationMet || !slices.Equal(v.Obligations[0].Evidence, []int64{11}) {
		t.Errorf("obligations %+v, want one met row citing [11]", v.Obligations)
	}
	if len(v.Conflicts) != 0 {
		t.Errorf("view conflicts %+v, want none for a resolved group", v.Conflicts)
	}
}

// TestStop_ConflictViewPairsWithUnmetRows: StopView.Conflicts pairs one to
// one, in order, with the unmet conflict rows of Obligations.
func TestStop_ConflictViewPairsWithUnmetRows(t *testing.T) {
	v := stopOf(&store.Run{}, nil, "s1", StopInput{Conflicts: []ConflictSeen{
		seen("protected_path", "p", unitA, 0, 2), seen("boundary_cross", "r", unitB, 6, 1), seen("boundary_cross", "r", unitA, 0, 4)}})
	var unmet []Obligation
	for _, o := range v.Obligations {
		if o.Unit != "" && o.State == ObligationUnmet {
			unmet = append(unmet, o)
		}
	}
	if len(unmet) != 2 || len(v.Conflicts) != 2 {
		t.Fatalf("unmet %+v view %+v, want two each", unmet, v.Conflicts)
	}
	for i := range unmet {
		if v.Conflicts[i].Unit != unmet[i].Unit || (v.Conflicts[i].RuleID != unmet[i].Source) {
			t.Errorf("pair %d: view %+v, row %+v", i, v.Conflicts[i], unmet[i])
		}
	}
	if v.Conflicts[0].Kind != "boundary_cross" || v.Conflicts[1].Kind != "protected_path" {
		t.Errorf("view order %+v, want the kinds' order", v.Conflicts)
	}
}

// TestLedger_ReplayWithConflicts (PLAN): EvaluateStop twice over one input,
// once with the conflicts JSON-encoded and decoded between, gives the same
// ledger and view.
func TestLedger_ReplayWithConflicts(t *testing.T) {
	in := []ConflictSeen{
		seen("signature_mismatch", "", unitA, 0, 8, 9), seen("boundary_cross", "r", unitB, 12, 3), seen("boundary_cross", "r", unitA, 0, 3),
		seen("protected_path", "p", unitA, 5, 2), seen("duplicate_abstraction", "", unitA, 0, 1),
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var back []ConflictSeen
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	r := runOf(t, edit(1, "pkg/a.go"))
	one, two := stopOf(r, nil, "s1", StopInput{Conflicts: in}), stopOf(r, nil, "s1", StopInput{Conflicts: back})
	if !reflect.DeepEqual(one.Obligations, two.Obligations) || !reflect.DeepEqual(one.Conflicts, two.Conflicts) {
		t.Errorf("replay differs:\n%+v\n%+v\n%+v\n%+v", one.Obligations, two.Obligations, one.Conflicts, two.Conflicts)
	}
	if len(one.Conflicts) != 2 {
		t.Errorf("view %+v, want the two unmet live groups", one.Conflicts)
	}
}

// TestVerify_ImportsNoGraph: internal/verify's module-local imports, walked
// transitively over its non-test files under the default tags, include no
// path under internal/graph (PLAN).
func TestVerify_ImportsNoGraph(t *testing.T) {
	root, mod := moduleRoot(t)
	seenPkg := map[string]bool{}
	var walk func(path string, via []string)
	walk = func(path string, via []string) {
		if seenPkg[path] {
			return
		}
		seenPkg[path] = true
		if path == mod+"/internal/graph" || strings.HasPrefix(path, mod+"/internal/graph/") {
			t.Errorf("internal/verify reaches %s through %s", path, strings.Join(via, " -> "))
			return
		}
		p, err := build.ImportDir(filepath.Join(root, strings.TrimPrefix(path, mod)), 0)
		if err != nil {
			t.Fatalf("import %s: %v", path, err)
		}
		for _, imp := range p.Imports {
			if imp == mod || strings.HasPrefix(imp, mod+"/") {
				walk(imp, append(via, path))
			}
		}
	}
	walk(mod+"/internal/verify", nil)
	if len(seenPkg) < 3 {
		t.Errorf("walked %d packages, want verify and its imports", len(seenPkg))
	}
}

// moduleRoot is the directory holding go.mod above the test's directory and
// the module path it names.
func moduleRoot(t *testing.T) (string, string) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for ; ; dir = filepath.Dir(dir) {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if m, ok := strings.CutPrefix(l, "module "); ok {
					return dir, strings.TrimSpace(m)
				}
			}
			t.Fatal("go.mod names no module")
		}
		if dir == filepath.Dir(dir) {
			t.Fatal("no go.mod above the test")
		}
	}
}
