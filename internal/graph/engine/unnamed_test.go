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
