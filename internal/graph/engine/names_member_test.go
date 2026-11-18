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
		// Gauge is declared in three units and Reset in two of them.
		{"north", "north/gauge.py", []synSym{{"type", "Gauge", ""}, fn("Reset")}},
		{"south", "south/gauge_reset.py", []synSym{{"type", "Gauge", ""}, fn("Reset")}},
		{"east", "east/gauge.py", []synSym{{"type", "Gauge", ""}}},
		// Meter is declared in two units; Spin is a free function beside
		// the first, not a member of Meter.
		{"gear", "gear/gear.py", []synSym{{"type", "Meter", ""}, fn("Spin")}},
		{"cog", "cog/cog.py", []synSym{{"type", "Meter", ""}}},
	}, nil, nil)
}

// wantNoUnitClaim is the line the brief finally states for the request:
// no unit line and no unit claimed from a line kept, the brief saying the
// unit is not known. The phrase stage may not claim a unit from the words
// left over once the named word has named none.
func wantNoUnitClaim(t *testing.T, g *graph.Store, b Brief) {
	t.Helper()
	c := claimOf(b, briefUnit(g, graph.RevCommit(graphtest.Commit), b))
	if c.Unit != "" || c.Line || !strings.Contains(b.Text(), "not known: the unit to change") {
		t.Errorf("brief %s, want no unit claimed and the unit stated as not known:\n%s", c, b.Text())
	}
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
	// The stated line, for a request whose only name is that word.
	b := Build(g, graph.RevCommit(graphtest.Commit), "Fix Tally.Missing so it counts every row.", Options{IDF: true})
	if n := b.Roles.Named; n.Unit != "" {
		t.Errorf("named unit %q, want none", n.Unit)
	}
	wantNoUnitClaim(t, g, b)
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

// (f) The type is in three units and the member in two. The rest of the
// request's words name one holder's path: that holder is the unit, as
// resolveIdent settles a bare name declared in several units.
func TestMember_TwoHoldersAreSettledByTheRestOfTheRequest(t *testing.T) {
	g := memberFixture(t)
	wantStated(t, g, "Fix Gauge.Reset in south so it counts every row.", "Gauge.Reset", NamedQualified, "south", "sym:py:south:func:gauge_reset.Reset")
	// The other holder, by its own word: the order of the index is not
	// what decides.
	wantStated(t, g, "Fix Gauge.Reset in north so it counts every row.", "Gauge.Reset", NamedQualified, "north", "sym:py:north:func:gauge.Reset")
	// A word that names the declarer without the member does not count: east
	// does not hold Reset.
	if nw := memberWord(t, g, "Gauge.Reset"); nw.Unit != "" {
		t.Errorf("word %+v, want no unit with nothing to tell the holders apart", nw)
	}
}

// (g) The same type and member, the rest of the words telling the two
// holders apart by nothing: no unit, the word still qualified, with the
// reason, and no unit stated for a request whose only name it is.
func TestMember_TwoHoldersTheWordsCannotSeparateNameNoUnit(t *testing.T) {
	g := memberFixture(t)
	nw := memberWord(t, g, "Gauge.Reset")
	if nw.Unit != "" || nw.Node != "" || !strings.Contains(nw.Why, "Reset is declared in 2 of the units that declare Gauge") {
		t.Errorf("word %+v, want no unit and Why naming Reset as declared in 2 of the units", nw)
	}
	b := Build(g, graph.RevCommit(graphtest.Commit), "Fix Gauge.Reset so it counts every row.", Options{IDF: true})
	if n := b.Roles.Named; n.Unit != "" {
		t.Errorf("named unit %q, want none", n.Unit)
	}
	wantNoUnitClaim(t, g, b)
}

// Known limit: a unit holds the member by a name match, not by the symbol
// belonging to the type. Spin is a free function declared beside Meter in
// gear; cog declares Meter and no Spin. Meter.Spin names gear, and the free
// function is the node. A change that checks membership changes this test.
func TestMember_KnownLimit_AFreeFunctionBesideTheTypeHoldsTheMember(t *testing.T) {
	g := memberFixture(t)
	wantStated(t, g, "Fix Meter.Spin so it counts every row.", "Meter.Spin", NamedQualified, "gear", "sym:py:gear:func:gear.Spin")
}
