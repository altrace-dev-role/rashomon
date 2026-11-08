package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestUnnamed_EachRuleReadsTheClaimAndTheRequestsNames (W-M1.38, the unit
// rule): E1 withholds a unit found with no unit named and no claim by a
// unit's own name; E3 adds that the request holds no unit's path or last
// segment and no file's base name; E4 withholds a unit claimed other than
// by name whose own path or last segment the request does not hold. Each
// name is matched case-folded with no letter or digit beside it, so a
// possessive or a hyphen bounds it and a plural does not; the root unit's
// "." names nothing. No rule withholds where no unit was found.
func TestUnnamed_EachRuleReadsTheClaimAndTheRequestsNames(t *testing.T) {
	c := corpus{
		unitID: map[string]graph.ID{"internal/platform": "unit:go:internal/platform", "cmd/rashomon": "unit:go:cmd/rashomon", ".": "unit:go:.",
			"src/Auth": "unit:java:src/Auth"},
		files: map[string]graph.ID{"internal/verify/read.go": "file:internal/verify/read.go", "build/Makefile": "file:build/Makefile"},
	}
	coverage := func() Roles {
		return Roles{Unit: "unit:go:internal/platform", Path: "internal/platform", Basis: BasisCoverage,
			Picks: []UnitPick{{Class: ClassFull}, {Class: ClassPartial}}}
	}
	named, unitPick, namedPick := coverage(), coverage(), coverage()
	named.Named.Unit = "internal/platform"
	unitPick.Picks[1].Class = ClassUnit
	namedPick.Picks[0].Class = ClassNamed
	for _, e := range []struct {
		name string
		r    Roles
		want bool
	}{
		{"no unit found", Roles{}, false},
		{"a unit named", named, false},
		{"a claim by a unit's name past the first pick", unitPick, false},
		{"a claim by a name", namedPick, false},
		{"word coverage alone", coverage(), true},
	} {
		if got := unitRuleE1(c, e.r, "make the read faster"); got != e.want {
			t.Errorf("E1, %s: %v, want %v", e.name, got, e.want)
		}
	}
	for _, e := range []struct {
		prompt string
		want   bool
	}{
		{"make the read faster", true},
		{"make the Platform read faster", false},
		{"make the platforms read faster", true},
		{"edit internal/platform so the read is faster", false},
		{"the read in Read.go is slow", false},
		{"the Makefile rule is slow", false},
		{"the auth flow is slow", false},
		{"the readgo path is slow", true},
		{"the rashomon-wide read is slow", false},
		{"make it faster . then stop", true},
	} {
		if got := unitRuleE3(c, coverage(), e.prompt); got != e.want {
			t.Errorf("E3, %q: %v, want %v", e.prompt, got, e.want)
		}
	}
	if unitRuleE3(c, named, "make the read faster") {
		t.Error("E3 withholds where E1 does not")
	}
	byName := coverage()
	byName.Basis = BasisUnitName
	byNamed := coverage()
	byNamed.Basis = BasisNamed
	for _, e := range []struct {
		name, prompt string
		r            Roles
		want         bool
	}{
		{"no unit found", "make the read faster", Roles{}, false},
		{"named", "make the read faster", byNamed, false},
		{"by its name, a plural", "the platforms read is slow", byName, true},
		{"by its name, the name", "the platform read is slow", byName, false},
		{"coverage, nothing named", "make the read faster", coverage(), true},
		{"coverage, a possessive", "the Platform's read is slow", coverage(), false},
		{"coverage, the path", "edit internal/platform", coverage(), false},
		{"coverage, another unit named", "the rashomon read is slow", coverage(), true},
	} {
		if got := unitRuleE4(c, e.r, e.prompt); got != e.want {
			t.Errorf("E4, %s: %v, want %v", e.name, got, e.want)
		}
	}
	if namesUnit("make it faster . then stop", ".") || namesUnit("go up .. and stop", "..") || namesUnit("make it faster . then stop", "") {
		t.Error("the root unit names something")
	}
	for _, e := range []struct {
		s, w string
		want bool
	}{
		{"graphs graph", "graph", true},
		{"graphs", "graph", false},
		{"graph", "graph", true},
		{"graph", "", false},
		{"the graph .", "", false},
		{"baseline-forgetting", "baseline", true},
		{"baseline-forgetting", "forgetting", true},
		{"summary's", "summary", true},
		{"graph2", "graph", false},
		{"2graph", "graph", false},
		{"Agraph", "graph", false},
		{"subgraph graph2 graph", "graph", true},
	} {
		if got := holdsWord(e.s, e.w); got != e.want {
			t.Errorf("holdsWord(%q, %q) = %v, want %v", e.s, e.w, got, e.want)
		}
	}
}

// TestUnnamed_AUnitTheRequestDoesNotNameIsNotKnown (W-M1.38): Task A's unit
// is claimed by word coverage and the request holds neither its path nor
// its last segment, so under E4 the unit is withheld with every role line,
// the decision says the request names no unit, and the not-known line is
// left alone; a brief that would say only what is not known is not sent.
func TestUnnamed_AUnitTheRequestDoesNotNameIsNotKnown(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true, unitRule: unitRuleE4})
	t.Logf("\n%s", b.Explain())
	auth := graph.UnitID("py", "services/auth")
	if !b.Roles.Found() || b.Roles.Unit != auth || b.Roles.Basis != BasisCoverage || !b.Roles.Unnamed {
		t.Fatalf("roles %+v", b.Roles)
	}
	if !b.Empty() || b.Kept != 0 || b.FirstKey() != "" || len(b.Lines) != 1 || b.Lines[0].Role != RoleNotKnown {
		t.Fatalf("kept %d of %+v", b.Kept, b.Lines)
	}
	all := []string{slot.UnknownUnit, RoleTarget, RoleReuse, RoleConsumer, RoleKnowledge}
	if nk, ok := b.Lines[0].Slot.NotKnown(); !ok || !slices.Equal(nk.Unknown, all) {
		t.Errorf("the not-known line %+v, want %v", nk, all)
	}
	if !slices.Contains(b.Decisions, Decision{ID: auth, Why: "withheld: role unit: the request names no unit"}) {
		t.Errorf("no decision says the request names no unit: %+v", b.Decisions)
	}
	if b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make services/auth session expiry respect the org-level timeout override.",
		Options{IDF: true, unitRule: unitRuleE4}); b.Roles.Unnamed || b.Empty() || strings.HasPrefix(b.Lines[0].Text, "not known") {
		t.Errorf("a request naming the unit's path: unnamed %v, kept %d", b.Roles.Unnamed, b.Kept)
	}
}

// TestUnnamed_E5KeepsWhatTheRequestNames (W-M1.40): E5 keeps a unit the
// request names, a qualified claim covering its whole phrase and a claim
// by the unit's own name in a phrase of at most two tokens, and withholds
// every other claim, whatever its score. A phrase is covered whole within
// a relative 1e-9, so the same weights summed in another order still
// cover it either way, and a claim short by 2e-9 of the phrase does not.
// The tolerance is pinned from both sides: a claim short by 0.5e-9 of the
// phrase is whole and one short by 1.5e-9 is not (short by 2e-9 rounds to
// just over 2e-9, so under a tolerance of 2e-9 it is not whole either).
func TestUnnamed_E5KeepsWhatTheRequestNames(t *testing.T) {
	x, y, z := 0.1, 0.2, 0.3
	lo, hi := z+y+x, x+y+z
	if lo == hi {
		t.Fatalf("%v and %v are equal: the last-bit cases test nothing", lo, hi)
	}
	claim := func(basis, class string, covered, of float64, target ...string) Roles {
		return Roles{Unit: "unit:go:internal/platform", Path: "internal/platform", Basis: basis, Target: target,
			Picks: []UnitPick{{Class: class, Covered: covered, Of: of}, {Class: ClassPartial, Covered: of / 2, Of: of}}}
	}
	two, three := []string{"platform", "read"}, []string{"platform", "read", "path"}
	for _, e := range []struct {
		name string
		r    Roles
		want bool
	}{
		{"no unit found", Roles{}, false},
		{"named, a long phrase", claim(BasisNamed, ClassNamed, 1, 1, three...), false},
		{"qualified, whole (T1's form)", claim(BasisCoverage, ClassQualified, 5.72, 5.72, two...), false},
		{"qualified, whole, a long phrase", claim(BasisCoverage, ClassQualified, 4, 4, three...), false},
		{"qualified, under by the last bit", claim(BasisCoverage, ClassQualified, lo, hi, three...), false},
		{"qualified, over by the last bit", claim(BasisCoverage, ClassQualified, hi, lo, three...), false},
		{"qualified, short by 2e-9 of the phrase", claim(BasisCoverage, ClassQualified, 4-2e-9*4, 4, three...), true},
		{"qualified, short by 1.5e-9 of the phrase", claim(BasisCoverage, ClassQualified, 4-1.5e-9*4, 4, three...), true},
		{"qualified, short by 0.5e-9 of the phrase", claim(BasisCoverage, ClassQualified, 4-0.5e-9*4, 4, three...), false},
		{"qualified, a word no name holds", claim(BasisCoverage, ClassQualified, 3, 4, three...), true},
		{"qualified, a phrase of no weight", claim(BasisCoverage, ClassQualified, 0, 0, two...), true},
		{"by its name, one token", claim(BasisUnitName, ClassUnit, 1, 1, "platform"), false},
		{"by its name, two tokens", claim(BasisUnitName, ClassUnit, 2, 2, two...), false},
		{"by its name, three tokens", claim(BasisUnitName, ClassUnit, 3, 3, three...), true},
		{"one name, whole", claim(BasisCoverage, ClassSingle, 2, 2, two...), true},
		{"two names, whole (Task A's form)", claim(BasisCoverage, ClassFull, 2, 2, two...), true},
		{"partial", claim(BasisCoverage, ClassPartial, 1, 2, two...), true},
	} {
		if got := unitRuleE5(corpus{}, e.r, "make the read faster"); got != e.want {
			t.Errorf("E5, %s: %v, want %v", e.name, got, e.want)
		}
	}
}

// TestUnnamed_TaskAIsSentWhenTheRequestNamesItsUnit (W-M1.40, PLAN 6.1):
// under the shipped rule Task A as written is not sent: its unit is a
// full claim, two of its names covering the phrase, and every line the
// brief holds is a role line withheld with the unit. A request that names
// the unit's path or a file of it claims the unit by name, and its brief
// names the purge job, the consumer outside the unit, and the lesson that
// applies to it.
func TestUnnamed_TaskAIsSentWhenTheRequestNamesItsUnit(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	rev := graph.RevCommit(graphtest.Commit)
	b := Build(sp.Store, rev, "make session expiry respect the org-level timeout override.", Options{IDF: true})
	if r := b.Roles; r.Why != ClassFull || r.Basis != BasisCoverage || !r.Unnamed || !b.Empty() || b.Kept != 0 {
		t.Fatalf("Task A as written: why %q basis %q unnamed %v, kept %d of %d lines\n%s", r.Why, r.Basis, r.Unnamed, b.Kept, len(b.Lines), b.Explain())
	}
	for _, prompt := range []string{
		"in services/auth, make session expiry respect the org-level timeout override.",
		"make session expiry in sessions.py respect the org-level timeout override.",
	} {
		b := Build(sp.Store, rev, prompt, Options{IDF: true})
		t.Logf("%s\nbrief:\n%s", prompt, b.Text())
		if r := b.Roles; r.Path != "services/auth" || r.Basis != BasisNamed || r.Unnamed || b.Empty() {
			t.Errorf("%q: unit %q basis %q unnamed %v, kept %d", prompt, r.Path, r.Basis, r.Unnamed, b.Kept)
			continue
		}
		for _, want := range []string{
			"\nunit 'services/auth' (named in the request) is consumed by 'jobs/purge_sessions.py'",
			"\nconsumer 'jobs/purge_sessions.py' imports 'auth.sessions', 'auth.conn'; INC-212 applies\n",
			"\nlesson INC-212 applies to 'jobs/purge_sessions.py': failure 'INC-212 session purge raced the TTL'; rule ",
		} {
			if !strings.Contains(b.Text(), want) {
				t.Errorf("%q: the brief has no %q", prompt, want)
			}
		}
	}
}
