package pgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const m = "example.test/mod"

func fixtureInitID(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testModule, "a", "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	// The id of a function named init carries its file and the byte offset of
	// its name, since several may share a package.
	return fmt.Sprintf("func:%s/a.init#a.go@%d", m, strings.Index(string(b), "func init")+len("func "))
}

func set(ts []Triple) map[Triple]bool {
	s := map[Triple]bool{}
	for _, x := range ts {
		s[x] = true
	}
	return s
}

func byPredicate(ts []Triple, p Predicate) []Triple {
	var out []Triple
	for _, x := range ts {
		if x.Predicate == p {
			out = append(out, x)
		}
	}
	return out
}

func equalSets(t *testing.T, what string, got []Triple, want []Triple) {
	t.Helper()
	g, w := set(got), set(want)
	for x := range w {
		if !g[x] {
			t.Errorf("%s: missing %v", what, x)
		}
	}
	for x := range g {
		if !w[x] {
			t.Errorf("%s: unexpected %v", what, x)
		}
	}
}

// Expected sets are written from the rules (static callee, Go-semantic embed,
// method-set satisfaction) applied to testdata/mod by hand, not read back
// from Extract.
func TestPL2_FixtureEdges(t *testing.T) {
	all := Extract(loadT(t, testModule))
	init := fixtureInitID(t)
	f := func(n string) string { return "func:" + m + "/a." + n }
	meth := func(recv, n string) string { return "method:(" + recv + ")." + n }
	A := m + "/a."

	equalSets(t, "calls", byPredicate(all, Calls), []Triple{
		{"func:" + A + "Total", Calls, meth(A+"Shape", "Area")}, // interface method: the named target, dispatch is CHA's
		{f("Describe"), Calls, meth(A+"Namer", "Name")},
		{init, Calls, f("Total")},
		{"pkg:" + m + "/a", Calls, f("New")}, // var initialiser
		{f("UseGen"), Calls, f("Gen")},       // two instantiations, one edge
		{f("UseBox"), Calls, meth(A+"Box[T]", "Get")},
		{f("Closure"), Calls, f("Helper")},  // closure body belongs to its declaration
		{f("Deferred"), Calls, f("Helper")}, // defer and go
		{meth(A+"Base", "Name"), Calls, "func:fmt.Sprint"},
		{"func:" + m + "/b.Run", Calls, f("Describe")},
	})
	equalSets(t, "embeds", byPredicate(all, Embeds), []Triple{
		{"type:" + A + "Square", Embeds, "type:" + A + "Base"},
		{"type:" + A + "Wrapper", Embeds, "type:" + A + "Base"}, // via the shared underlying struct
		{"type:" + A + "RW", Embeds, "type:" + A + "Shape"},
		{"type:" + A + "RW", Embeds, "type:fmt.Stringer"},
		{"type:" + m + "/b.Tri", Embeds, "type:" + A + "Base"},
	})
	ty := func(p, n string) string { return "type:" + m + "/" + p + "." + n }
	equalSets(t, "implements", byPredicate(all, Implements), []Triple{
		{ty("a", "Base"), Implements, ty("a", "Namer")},
		{ty("a", "Square"), Implements, ty("a", "Namer")},
		{ty("a", "Circle"), Implements, ty("a", "Shape")},
		{ty("a", "Circle"), Implements, ty("a", "Namer")},
		{ty("a", "Circle"), Implements, ty("b", "Local")},
		{ty("a", "Wrapper"), Implements, ty("a", "Namer")},
		{ty("b", "Tri"), Implements, ty("a", "Shape")}, // cross-package: a.Base.Name promoted
		{ty("b", "Tri"), Implements, ty("a", "Namer")},
		{ty("b", "Tri"), Implements, ty("b", "Local")},
	})
	equalSets(t, "implements-via-pointer", byPredicate(all, ImplementsViaPointer), []Triple{
		{ty("a", "Square"), ImplementsViaPointer, ty("a", "Shape")},
		{ty("a", "Square"), ImplementsViaPointer, ty("b", "Local")},
	})

	// ACCEPTED GAP: generic types are never implementers. Box[T] has a Name
	// method and would satisfy Namer if instantiated, but the extractor skips
	// uninstantiated generic types, so no implements fact names it.
	for _, x := range all {
		if x.Subject == ty("a", "Box") && x.Predicate != Declares {
			t.Errorf("generic type took part in a relation other than declares: %v", x)
		}
	}

	// Declarations: must contain, and must not contain.
	d := set(byPredicate(all, Declares))
	for _, w := range []Triple{
		{"pkg:" + m + "/a", Declares, ty("a", "Shape")},
		{"pkg:" + m + "/a", Declares, ty("a", "Alias")},
		{"pkg:" + m + "/a", Declares, ty("a", "AliasSq")},
		{"pkg:" + m + "/a", Declares, ty("a", "Box")},
		{"pkg:" + m + "/a", Declares, f("Total")},
		{"pkg:" + m + "/a", Declares, "var:" + A + "Default"},
		{"pkg:" + m + "/a", Declares, init},
		{"pkg:" + m + "/b", Declares, ty("b", "Tri")},
		{ty("a", "Base"), Declares, meth(A+"Base", "Name")},
		{ty("a", "Square"), Declares, meth("*"+A+"Square", "Area")},
		{ty("a", "Box"), Declares, meth(A+"Box[T]", "Get")},
		{ty("b", "Tri"), Declares, meth(m+"/b.Tri", "Area")},
	} {
		if !d[w] {
			t.Errorf("declares: missing %v", w)
		}
	}
	for _, x := range all {
		for _, banned := range []string{"Skipped", "TestX", "mod/c"} {
			if strings.Contains(x.Subject, banned) || strings.Contains(x.Object, banned) {
				t.Errorf("outside the build configuration: %v", x)
			}
		}
	}
	// A call through a func value and a conversion are not edges.
	for _, x := range byPredicate(all, Calls) {
		if strings.Contains(x.Subject, "Dyn") || strings.Contains(x.Subject, "Conv") {
			t.Errorf("non-static call recorded: %v", x)
		}
	}
}

func TestPL2_ExtractDeterministicSortedUnique(t *testing.T) {
	p := loadT(t, testModule)
	a, b := Extract(p), Extract(p)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("lens %d %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("run differs at %d", i)
		}
		if i > 0 && !tripleLess(a[i-1], a[i]) {
			t.Fatalf("not strictly sorted at %d: %v %v", i, a[i-1], a[i])
		}
	}
}

// Every edge agrees with an independent re-derivation (oracle_test.go) on the
// fixture module and on this repository's own checkout.
func TestPL2_EdgesAgreeWithRederivation(t *testing.T) {
	for name, root := range map[string]string{"testdata": testModule, "repo": ""} {
		t.Run(name, func(t *testing.T) {
			if name == "repo" {
				root = repoRoot(t)
			}
			p := loadT(t, root)
			got, want := Extract(p), rederive(t, p)
			counts := map[Predicate]int{}
			for _, x := range got {
				counts[x.Predicate]++
			}
			// Guard against two empty answers agreeing.
			for _, pr := range []Predicate{Declares, Calls, Embeds, Implements} {
				if counts[pr] == 0 {
					t.Errorf("no %v facts extracted", pr)
				}
			}
			diffTriples(t, got, want)
			t.Logf("%s: %d facts agree (%v)", name, len(got), counts)
		})
	}
}

func diffTriples(t *testing.T, got, want []Triple) {
	t.Helper()
	g, w := set(got), set(want)
	n := 0
	for x := range g {
		if !w[x] && n < 10 {
			t.Errorf("extractor only: %v", x)
			n++
		}
	}
	for x := range w {
		if !g[x] && n < 20 {
			t.Errorf("oracle only: %v", x)
			n++
		}
	}
	if len(g) != len(w) {
		t.Errorf("distinct counts: extractor %d oracle %d", len(g), len(w))
	}
}
