package lang

import (
	"errors"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// fake is a resolver of language l claiming the files ending in ext; at is
// the root its configuration step made it for, "" as registered.
type fake struct{ l, ext, at string }

func (f fake) Lang() string              { return f.l }
func (f fake) Claims(_, rel string) bool { return strings.HasSuffix(rel, f.ext) }
func (f fake) Unit(_, rel string) (graph.ID, bool) {
	return graph.UnitID(f.l, path.Dir(rel)), true
}
func (fake) Extract(string, []string, graph.Rev) ([]graph.Record, error)      { return nil, nil }
func (fake) Overlay(string, []byte, graph.Rev, int64) ([]graph.Record, error) { return nil, nil }
func (fake) OverlayCarried() graph.Carried                                    { return nil }

// errRefused is the configuration step's refusal of a root.
var errRefused = errors.New("refused")

// counted is a configuration step for f that makes f for the root it runs
// under, refusing the roots in refuse, and counts its runs per root.
func counted(f fake, runs map[string]int, refuse ...string) func(string) (Resolver, error) {
	return func(root string) (Resolver, error) {
		runs[root]++
		if slices.Contains(refuse, root) {
			return nil, errRefused
		}
		f.at = root
		return f, nil
	}
}

// TestRegistry_ClaimConfiguresOncePerRoot (once per entry and root):
// two Claims under one root run the step once and return the same
// resolver; a Claim under a second root runs it again, for that root, and
// gets that root's resolver, not the first's. A copy of the registry by
// value shares the runs. Break: run the step on every Claim, or keep
// one result per entry whatever the root.
func TestRegistry_ClaimConfiguresOncePerRoot(t *testing.T) {
	runs := map[string]int{}
	var g Registry
	g.RegisterConfigured(fake{l: "py", ext: ".py"}, counted(fake{l: "py", ext: ".py"}, runs))
	copied := g
	for _, c := range []struct {
		reg       *Registry
		root, rel string
	}{{&g, "/a", "x.py"}, {&copied, "/a", "y/z.py"}, {&g, "/b", "x.py"}, {&g, "/a", "x.py"}} {
		r, ok, err := c.reg.Claim(c.root, c.rel)
		if !ok || err != nil {
			t.Fatalf("Claim(%s, %s) = %v, %v, %v; want the resolver", c.root, c.rel, r, ok, err)
		}
		if got := r.(fake).at; got != c.root {
			t.Errorf("Claim(%s, %s) returned the resolver configured for %q", c.root, c.rel, got)
		}
	}
	if want := map[string]int{"/a": 1, "/b": 1}; !maps.Equal(runs, want) {
		t.Errorf("the step ran %v, want once per root %v", runs, want)
	}
}

// TestRegistry_TwoRootsOneReadOneRefused (once per entry and root): in one
// process one root's configuration reads and the other's is refused; each
// Claim returns its own root's resolver or error, every time, and the order
// of the two roots changes neither. Break: return the refusal on the first
// Claim only, or share one result across roots.
func TestRegistry_TwoRootsOneReadOneRefused(t *testing.T) {
	for _, order := range [][]string{{"/read", "/refused"}, {"/refused", "/read"}} {
		runs := map[string]int{}
		var g Registry
		g.RegisterConfigured(fake{l: "py", ext: ".py"}, counted(fake{l: "py", ext: ".py"}, runs, "/refused"))
		for range 2 {
			for _, root := range order {
				r, ok, err := g.Claim(root, "m.py")
				switch {
				case !ok:
					t.Fatalf("order %v: Claim under %s claimed nothing", order, root)
				case root == "/refused" && (r != nil || !errors.Is(err, errRefused)):
					t.Errorf("order %v: Claim under %s = %v, %v; want nil and the refusal", order, root, r, err)
				case root == "/read" && (err != nil || r == nil || r.(fake).at != "/read"):
					t.Errorf("order %v: Claim under %s = %v, %v; want the resolver for /read", order, root, r, err)
				}
			}
		}
		if want := map[string]int{"/read": 1, "/refused": 1}; !maps.Equal(runs, want) {
			t.Errorf("order %v: the step ran %v, want %v", order, runs, want)
		}
	}
}

// TestRegistry_UnclaimedRunsNoStep: a path no resolver claims is
// false with no error, and no step runs, before or after it; nor does
// registering run one. Break: run the steps when registered or when any
// path is asked for.
func TestRegistry_UnclaimedRunsNoStep(t *testing.T) {
	runs := map[string]int{}
	var g Registry
	g.RegisterConfigured(fake{l: "go", ext: ".go"}, counted(fake{l: "go", ext: ".go"}, runs))
	g.RegisterConfigured(fake{l: "py", ext: ".py"}, counted(fake{l: "py", ext: ".py"}, runs))
	if len(runs) != 0 {
		t.Fatalf("registering ran the steps: %v", runs)
	}
	if r, ok, err := g.Claim("/a", "notes.md"); r != nil || ok || err != nil {
		t.Errorf("Claim of an unclaimed path = %v, %v, %v; want nil, false, nil", r, ok, err)
	}
	if len(runs) != 0 {
		t.Errorf("an unclaimed path ran the steps: %v", runs)
	}
	if _, _, err := g.Claim("/a", "m.py"); err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"/a": 1}; !maps.Equal(runs, want) {
		t.Errorf("a claimed .py path ran %v, want only its own step once %v", runs, want)
	}
}

// TestRegistry_ClaimWithNoStepIsTheRegisteredResolver: Register keeps its
// meaning; Claim and For both return the resolver as registered, and the
// first registered resolver that claims a path wins.
func TestRegistry_ClaimWithNoStepIsTheRegisteredResolver(t *testing.T) {
	first, second := fake{l: "go", ext: ".go", at: "registered"}, fake{l: "other", ext: ".go"}
	var g Registry
	g.Register(first)
	g.Register(second)
	if r, ok, err := g.Claim("/a", "x.go"); r != first || !ok || err != nil {
		t.Errorf("Claim = %v, %v, %v; want the first registered resolver", r, ok, err)
	}
	if r, ok := g.For("/a", "x.go"); r != first || !ok {
		t.Errorf("For = %v, %v; want the first registered resolver", r, ok)
	}
}

// TestRegistry_LangsInRegistrationOrder: the languages as registered,
// each once, in the order of their first registration. Break: sort them.
func TestRegistry_LangsInRegistrationOrder(t *testing.T) {
	var g Registry
	g.Register(fake{l: "py", ext: ".py"})
	g.RegisterConfigured(fake{l: "go", ext: ".go"}, counted(fake{l: "go"}, map[string]int{}))
	g.Register(fake{l: "py", ext: ".pyi"})
	if got, want := g.Langs(), []string{"py", "go"}; !slices.Equal(got, want) {
		t.Errorf("Langs() = %v, want %v", got, want)
	}
	var none Registry
	if got := none.Langs(); len(got) != 0 {
		t.Errorf("an empty registry's Langs() = %v, want none", got)
	}
}
