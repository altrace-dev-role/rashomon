package engine

import (
	"maps"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// The signature fixtures: two Python units, lib and app, each its own top
// package, indexed by pylite at C1. A case edits one def line through
// pylite's Overlay at seq 1 and reads the signature_mismatch conflicts of
// the Diff of C1 to that overlay over the edited file, under no rule, so
// each case is what the post's check sees for one Python edit.

// sigRes is the fixtures' resolver.
func sigRes() pylite.Resolver {
	return pylite.Resolver{
		Units:   []pylite.Unit{{Path: "lib"}, {Path: "app"}},
		Modules: map[string]string{"lib": "lib", "app": "app"},
	}
}

// sigSpecimen writes files under a temp dir and indexes every one at C1,
// as graphtest.New does for its specimen.
func sigSpecimen(t *testing.T, files map[string]string) *graphtest.Specimen {
	t.Helper()
	root := t.TempDir()
	sp := &graphtest.Specimen{Store: graph.NewStore(), Files: map[string][]byte{}, Root: root, Res: sigRes()}
	rels := make([]string, 0, len(files))
	for rel, body := range files {
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		sp.Files[rel] = []byte(body)
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	recs, err := sp.Res.Extract(root, rels, graph.RevCommit(graphtest.Commit))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Node != nil {
			sp.Store.AssertNode(*r.Node)
		}
		if r.Edge != nil {
			sp.Store.AssertEdge(*r.Edge)
		}
	}
	return sp
}

// sigEdit replaces old with new in rel as an overlay at seq 1 and returns
// the signature_mismatch conflicts the edit raises.
func sigEdit(t *testing.T, sp *graphtest.Specimen, rel, old, new string) []Conflict {
	t.Helper()
	if _, err := sp.Overlay(rel, old, new, 1); err != nil {
		t.Fatal(err)
	}
	c1, ov := graph.RevCommit(graphtest.Commit), graph.RevOverlay(graphtest.Commit, 1)
	return onlyKind(Conflicts(sp.Store, DiffOf(sp.Store, c1, ov, []graph.ID{graph.FileID(rel)}), nil), ConflictSignatureMismatch)
}

// sigGot is each conflict's caller and the count it passes, so a case
// names the callers it expects whatever their order.
func sigGot(cs []Conflict) map[graph.ID]int {
	m := map[graph.ID]int{}
	for _, c := range cs {
		m[c.Edge.Src] = c.Got
	}
	return m
}

// sigCheck reports every conflict that is not on callee, in unit, with
// want as the signature it names, and no rule.
func sigCheck(t *testing.T, cs []Conflict, callee, unit graph.ID, want string) {
	t.Helper()
	for _, c := range cs {
		if c.Callee.ID != callee || c.Edge.Dst != callee || c.Subject != callee || c.Unit != unit || c.Want != want || c.RuleID != "" {
			t.Errorf("conflict %+v: want callee and subject %s, unit %s, signature %q, no rule", c, callee, unit, want)
		}
	}
}

func pyFunc(unit, qualified string) graph.ID {
	return graph.SymID(pylite.Lang, unit, "func", qualified)
}

func pyMethod(unit, qualified string) graph.ID {
	return graph.SymID(pylite.Lang, unit, "method", qualified)
}

// TestSignature_KeywordCallPastTheBoundConflicts: a keyword argument fills
// one parameter as a positional one does, so a call whose count leaves the
// new bounds raises a TypeError in Python whatever its keywords: over the
// new arity_max when parameters go (an unexpected keyword), under the new
// arity_min when a keyword-only parameter with no default comes after a
// bare star (a missing keyword-only argument). The bare star counts in
// neither bound, and a keyword-only parameter counts in both: full, which
// passes three, fits (x, factor, *, mode, unit=1). In each case the caller
// whose count fits the new bounds raises nothing.
func TestSignature_KeywordCallPastTheBoundConflicts(t *testing.T) {
	const calcPy, usePy = "lib/calc.py", "app/use.py"
	const oldDef = "def scale(x, factor=2, mode=0):"
	scale := pyFunc("lib", "lib.calc.scale")
	big, full, small := pyFunc("app", "app.use.big"), pyFunc("app", "app.use.full"), pyFunc("app", "app.use.small")
	for _, tc := range []struct {
		name, newDef, sig string
		want              map[graph.ID]int
	}{
		{"parameters go", "def scale(x):", "(x)", map[graph.ID]int{big: 2, full: 3}},
		{"a keyword-only parameter comes", "def scale(x, factor, *, mode, unit=1):", "(x, factor, mode, unit)", map[graph.ID]int{big: 2, small: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := sigSpecimen(t, map[string]string{
				calcPy: oldDef + "\n    return x\n",
				usePy: `from lib.calc import scale


def big(v):
    return scale(v, factor=3)


def full(v):
    return scale(v, factor=1, mode=0)


def small(v):
    return scale(x=v)
`,
			})
			if c := ArityConflicts(sp.Store, scale, graph.RevCommit(graphtest.Commit)); len(c) != 0 {
				t.Fatalf("precondition: every call fits %s at C1, got %s", oldDef, describe(c))
			}
			got := sigEdit(t, sp, calcPy, oldDef, tc.newDef)
			if !maps.Equal(sigGot(got), tc.want) {
				t.Errorf("%s: %s, want callers and counts %v", tc.newDef, cstr(got), tc.want)
			}
			sigCheck(t, got, scale, graph.UnitID(pylite.Lang, "lib"), tc.sig)
		})
	}
}

// TestSignature_SelfCallToAMethodConflicts: self.load(...) in a method of
// Store resolves to Store.load, and both counts leave self out: the def's
// bounds skip the parameter and the call's count never holds it. So the
// edit that makes default required and adds strict raises one conflict, on
// get, which passes one argument, and none on peek, which passes two. Were
// self in either count, C1 would already hold an arity conflict or peek
// would raise one.
func TestSignature_SelfCallToAMethodConflicts(t *testing.T) {
	const storePy = "lib/store.py"
	const oldDef, newDef = "def load(self, key, default=None):", "def load(self, key, default, strict=False):"
	sp := sigSpecimen(t, map[string]string{storePy: `class Store:
    ` + oldDef + `
        return key

    def get(self, key):
        return self.load(key)

    def peek(self, key):
        return self.load(key, 0)
`})
	load := pyMethod("lib", "lib.store.Store.load")
	c1 := graph.RevCommit(graphtest.Commit)
	for _, caller := range []graph.ID{pyMethod("lib", "lib.store.Store.get"), pyMethod("lib", "lib.store.Store.peek")} {
		if _, ok := sp.Store.Edge(caller, graph.Calls, load, c1); !ok {
			t.Fatalf("precondition: no calls edge %s -> %s at C1", caller, load)
		}
	}
	if c := ArityConflicts(sp.Store, load, c1); len(c) != 0 {
		t.Fatalf("precondition: both self calls fit %s at C1, got %s", oldDef, describe(c))
	}
	got := sigEdit(t, sp, storePy, oldDef, newDef)
	if want := map[graph.ID]int{pyMethod("lib", "lib.store.Store.get"): 1}; !maps.Equal(sigGot(got), want) {
		t.Errorf("%s: %s, want only get with one argument", newDef, cstr(got))
	}
	sigCheck(t, got, load, graph.UnitID(pylite.Lang, "lib"), "(self, key, default, strict)")
}

// TestSignature_AliasedCalleeConflicts: a callee imported under another
// name (from lib.calc import add as plus) is the def's own symbol, so the
// edit that adds a parameter raises the conflict on the caller that calls
// it as plus, in the callee's unit, across units.
func TestSignature_AliasedCalleeConflicts(t *testing.T) {
	const calcPy = "lib/calc.py"
	const oldDef, newDef = "def add(a, b):", "def add(a, b, c):"
	sp := sigSpecimen(t, map[string]string{
		calcPy: oldDef + "\n    return a\n",
		"app/main.py": `from lib.calc import add as plus


def run(a, b):
    return plus(a, b)
`,
	})
	add, run := pyFunc("lib", "lib.calc.add"), pyFunc("app", "app.main.run")
	if _, ok := sp.Store.Edge(run, graph.Calls, add, graph.RevCommit(graphtest.Commit)); !ok {
		t.Fatalf("precondition: the aliased call does not reach %s at C1", add)
	}
	got := sigEdit(t, sp, calcPy, oldDef, newDef)
	if want := map[graph.ID]int{run: 2}; !maps.Equal(sigGot(got), want) {
		t.Errorf("%s: %s, want run with two arguments", newDef, cstr(got))
	}
	sigCheck(t, got, add, graph.UnitID(pylite.Lang, "lib"), "(a, b, c)")
}

// TestSignature_ConstructorCallRaisesNone: a class called as a constructor
// raises no conflict, though Store(path) fails in Python once __init__
// takes a mode. The index writes no class node, so the call ends at the
// class id, which carries no arity, and the __init__ whose signature
// changed has no caller. What a class call admits is what its __new__,
// __init__ and metaclass admit; the preconditions fail first if the index
// starts to write class nodes, and that case is then to be ruled before a
// line says it.
func TestSignature_ConstructorCallRaisesNone(t *testing.T) {
	const modelsPy = "lib/models.py"
	const oldDef, newDef = "def __init__(self, path):", "def __init__(self, path, mode):"
	sp := sigSpecimen(t, map[string]string{
		modelsPy: "class Store:\n    " + oldDef + "\n        self.path = path\n",
		"app/start.py": `from lib.models import Store


def boot(path):
    return Store(path)
`,
	})
	class := graph.SymID(pylite.Lang, "lib", "class", "lib.models.Store")
	ctor := pyMethod("lib", "lib.models.Store.__init__")
	c1, ov := graph.RevCommit(graphtest.Commit), graph.RevOverlay(graphtest.Commit, 1)
	if _, ok := sp.Store.Edge(pyFunc("app", "app.start.boot"), graph.Calls, class, c1); !ok {
		t.Fatalf("precondition: the constructor call does not end at %s at C1", class)
	}
	got := sigEdit(t, sp, modelsPy, oldDef, newDef)
	if n, ok := sp.Store.Node(class, ov); ok {
		t.Fatalf("precondition: the index wrote a class node %+v", n)
	}
	if n, ok := sp.Store.Node(ctor, ov); !ok || n.Attrs["sig"] != "(self, path, mode)" {
		t.Fatalf("precondition: __init__ at the overlay is %+v, %v; want sig (self, path, mode)", n, ok)
	}
	if len(got) != 0 {
		t.Errorf("a constructor call: %s, want none", cstr(got))
	}
}

// TestSignature_CallWithNoCountRaisesNone: a calls edge with no argc, the
// edge an index writes for a call whose count it cannot vouch for, raises
// no signature_mismatch on an edited callee, while a caller of the same
// callee whose argc leaves the new bounds still raises one. A missing argc
// is no count, not zero.
func TestSignature_CallWithNoCountRaisesNone(t *testing.T) {
	f := newFx()
	fl := f.file("a", "a/one.lx", fxC1)
	s := f.sym(fl, "a", "func", "S", fxC1, map[string]string{"sig": "(a)", "arity_min": "1", "arity_max": "1"})
	counted := f.sym(fl, "a", "func", "Counted", fxC1, nil)
	uncounted := f.sym(fl, "a", "func", "Uncounted", fxC1, nil)
	f.edge(counted, graph.Calls, s, fxC1, map[string]string{"argc": "1", "line": "3"})
	f.edge(uncounted, graph.Calls, s, fxC1, map[string]string{"line": "5"})
	ov := fxOv(1)
	f.s.AssertNode(graph.Node{ID: s, Kind: "sym", Rev: ov,
		Attrs: map[string]string{"name": "S", "symkind": "func", "sig": "(a, b)", "arity_min": "2", "arity_max": "2"}})
	got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fl}), nil), ConflictSignatureMismatch)
	if want := map[graph.ID]int{counted: 1}; !maps.Equal(sigGot(got), want) {
		t.Errorf("%s, want only the counted caller", cstr(got))
	}
}

// TestSignature_CalleeWithNoArityRaisesNone: a callee whose sig changes but
// which carries no arity at the edit, as an index writes a def whose bounds
// it cannot vouch for, raises no signature_mismatch however its callers
// count, and one bound alone is no arity. The control is the same edit
// with both bounds, which raises one.
func TestSignature_CalleeWithNoArityRaisesNone(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bounds map[string]string
		want   int
	}{
		{"no bound", nil, 0},
		{"arity_min alone", map[string]string{"arity_min": "2"}, 0},
		{"arity_max alone", map[string]string{"arity_max": "2"}, 0},
		{"both bounds", map[string]string{"arity_min": "2", "arity_max": "2"}, 1},
	} {
		f := newFx()
		fl := f.file("a", "a/one.lx", fxC1)
		s := f.sym(fl, "a", "func", "S", fxC1, map[string]string{"sig": "(a)", "arity_min": "1", "arity_max": "1"})
		caller := f.sym(fl, "a", "func", "T", fxC1, nil)
		f.edge(caller, graph.Calls, s, fxC1, map[string]string{"argc": "1"})
		ov := fxOv(1)
		attrs := map[string]string{"name": "S", "symkind": "func", "sig": "(a, b)"}
		maps.Copy(attrs, tc.bounds)
		f.s.AssertNode(graph.Node{ID: s, Kind: "sym", Rev: ov, Attrs: attrs})
		if got := onlyKind(Conflicts(f.s, DiffOf(f.s, fxC1, ov, []graph.ID{fl}), nil), ConflictSignatureMismatch); len(got) != tc.want {
			t.Errorf("%s: %s, want %d", tc.name, cstr(got), tc.want)
		}
	}
}
