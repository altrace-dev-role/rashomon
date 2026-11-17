package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// QUAL-MEMBER1: Type.member names the unit that declares the member. A
// type declared in several units named the first one met, whether or not
// that one held the member (the development rows NN17, Tally.Kept, and
// NN19, File.Prompts: the brief stated the first declarer, the member was
// in another). One fixture, one case per branch of the rule.

func memberFixture(t *testing.T) *graph.Store {
	t.Helper()
	return synth(t, []synFile{
		{"learn", "learn/tally.py", []synSym{{"type", "Tally", ""}, fn("Total")}},
		{"seed", "seed/tally.py", []synSym{{"type", "Tally", ""}, fn("Kept")}},
		{"fold", "fold/fold.py", []synSym{{"type", "File", ""}, fn("Gather")}},
		{"spec", "spec/spec.py", []synSym{{"type", "File", ""}, {"type", "fold", ""}}},
		// Ledger is declared twice in one unit: one declaring unit, not two.
		{"plan", "plan/plan.py", []synSym{{"type", "Ledger", ""}}},
		{"plan", "plan/more.py", []synSym{{"type", "Ledger", ""}}},
	}, nil, nil)
}

func memberWord(t *testing.T, g *graph.Store, w string) NamedWord {
	t.Helper()
	nw, ok := identWord(g, "Fix "+w+" so it counts every row.", w)
	if !ok || nw.Kind != NamedQualified {
		t.Fatalf("%q: named %v %+v, want a qualified word", w, ok, nw)
	}
	return nw
}

// (a) The type is in two units and the member only in the second: the
// second is the unit, with the member as the node.
func TestMember_ATypeInTwoUnitsNamesTheOneThatHoldsTheMember(t *testing.T) {
	g := memberFixture(t)
	wantStated(t, g, "Fix Tally.Kept so it counts every row.", "Tally.Kept", NamedQualified, "seed", "sym:py:seed:func:tally.Kept")
	// The first unit holds the member here: the order of declaration is
	// not what decides.
	wantStated(t, g, "Fix Tally.Total so it counts every row.", "Tally.Total", NamedQualified, "learn", "sym:py:learn:func:tally.Total")
}

// (b) The type is in two units and the member in neither: the word names
// no unit, and is still a qualified word with the reason.
func TestMember_ATypeInTwoUnitsWithoutTheMemberNamesNoUnit(t *testing.T) {
	g := memberFixture(t)
	nw := memberWord(t, g, "Tally.Missing")
	if nw.Unit != "" || nw.Node != "" || !strings.Contains(nw.Why, "Missing") || !strings.Contains(nw.Why, "none of") {
		t.Errorf("word %+v, want no unit and Why naming Missing as declared in none of the units", nw)
	}
	n := Build(g, graph.RevCommit(graphtest.Commit), "Fix Tally.Missing so it counts every row.", Options{IDF: true}).Roles.Named
	if n.Unit != "" {
		t.Errorf("named unit %q, want none", n.Unit)
	}
}

// (c) The type is in one unit and the member is not indexed (a field, an
// unexported member): that unit, no node, as before.
func TestMember_ATypeInOneUnitKeepsItsUnitWhenTheMemberIsNotIndexed(t *testing.T) {
	g := memberFixture(t)
	nw := memberWord(t, g, "Ledger.Balance")
	if nw.Unit != "plan" || nw.Node != "" || !strings.Contains(nw.Why, "Balance is not declared in plan") {
		t.Errorf("word %+v, want plan, no node, and the member's absence disclosed", nw)
	}
}

// (d) A unit-segment qualifier is read first and unchanged: fold.Gather is
// the fold unit though a type named fold is declared in spec, and a member
// fold does not hold keeps the unit it named.
func TestMember_AUnitSegmentQualifierIsUnchanged(t *testing.T) {
	g := memberFixture(t)
	wantStated(t, g, "Fix fold.Gather so it counts every row.", "fold.Gather", NamedQualified, "fold", "sym:py:fold:func:fold.Gather")
	nw := memberWord(t, g, "fold.Nothing")
	if nw.Unit != "fold" || nw.Node != "" || !strings.Contains(nw.Why, "Nothing is not declared in fold") {
		t.Errorf("word %+v, want fold with the member's absence disclosed", nw)
	}
}

// (e) The positive twin of (b): on the same graph a member that is
// declared resolves, so (b)'s empty unit is the rule and not a word that
// never reads. File is declared in two units, as NN19's File is in four.
func TestMember_APositiveTwinOfTheUndeclaredMember(t *testing.T) {
	g := memberFixture(t)
	wantStated(t, g, "Fix File.Gather so it counts every row.", "File.Gather", NamedQualified, "fold", "sym:py:fold:func:fold.Gather")
	if nw := memberWord(t, g, "File.Prompts"); nw.Unit != "" {
		t.Errorf("File.Prompts: word %+v, want no unit when no declarer of File holds Prompts", nw)
	}
}
