package gopg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// TestOverlay_SymDocEqualsIndexDoc: for every symbol of the shapes fixture's
// files, the overlay's doc attr equals the doc attr the index writes for the
// same id, read from the snapshot through the mapper, never typed here.
func TestOverlay_SymDocEqualsIndexDoc(t *testing.T) {
	root := shapes(t)
	snap, err := platform.ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	documented, undocumented := 0, 0
	for _, rel := range []string{"shape/shape.go", "app/app.go"} {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		idx, _ := split(newMapper(snap, []string{rel}, graph.RevCommit("C1")).records())
		compared := 0
		for _, r := range overlayOf(t, rel, string(src), 1) {
			if r.Node == nil || r.Node.Kind != "sym" {
				continue
			}
			want, ok := idx[r.Node.ID]
			if !ok {
				t.Errorf("%s: %s is not in the index", rel, r.Node.ID)
				continue
			}
			compared++
			got, gotOK := r.Node.Attrs["doc"]
			wantDoc, wantOK := want.Attrs["doc"]
			if got != wantDoc || gotOK != wantOK {
				t.Errorf("%s: doc of %s = %q (present %v), index %q (present %v)", rel, r.Node.ID, got, gotOK, wantDoc, wantOK)
			}
			if wantDoc != "" {
				documented++
			} else {
				undocumented++
			}
		}
		if compared == 0 {
			t.Errorf("%s: no symbol compared", rel)
		}
	}
	if documented == 0 {
		t.Errorf("no compared symbol has a doc: both sides could be empty")
	}
	if undocumented == 0 {
		t.Errorf("no compared symbol lacks a doc: the no-attr case is unproven")
	}
}

func overlayNode(t *testing.T, src string, id graph.ID) graph.Node {
	t.Helper()
	nodes, _ := split(overlayOf(t, "p/p.go", src, 1))
	n, ok := nodes[id]
	if !ok {
		t.Fatalf("no node %s", id)
	}
	return n
}

// TestOverlay_SymDocFollowsPgraphSynopsis: the cases the shapes fixture may
// lack, each mirroring a pgraph/symbols.go rule. Shapes already holds the
// single-spec GenDecl (Square, Max: pgraph/symbols.go:111-112, equality
// pinned above) and the undocumented symbol (Default); the over-long cut,
// the non-ASCII drop and the multi-spec GenDecl are typed here.
func TestOverlay_SymDocFollowsPgraphSynopsis(t *testing.T) {
	long := strings.Repeat("word ", 40)
	src := "package p\n\n" +
		"// Long " + long + "ends here.\nfunc Long() {}\n\n" +
		"// Café serves ☃ tea.\nfunc Cafe() {}\n\n" +
		"// Group doc is not the spec's.\nconst (\n\tA = 1\n\t// B has its own doc.\n\tB = 2\n)\n\n" +
		"// Single is documented by its declaration.\ntype Single struct{}\n\n" +
		"func Bare() {}\n"
	id := func(kind, q string) graph.ID { return graph.SymID(Lang, "p", kind, q) }
	// pgraph/symbols.go:138-157: first sentence, then cut to 160 at a space.
	l := overlayNode(t, src, id("func", "Long")).Attrs["doc"]
	if l == "" || len(l) > MaxDocBytes || strings.HasSuffix(l, " ") || strings.Contains(l, "ends") {
		t.Errorf("long doc = %q (%d bytes), want a non-empty cut within %d at a word boundary", l, len(l), MaxDocBytes)
	}
	if !strings.HasPrefix("Long "+long, l) || strings.HasSuffix(l, "wor") {
		t.Errorf("long doc %q is not a whole-word prefix", l)
	}
	// pgraph/symbols.go:142-146: only runes 0x20..0x7e stay.
	if got, want := overlayNode(t, src, id("func", "Cafe")).Attrs["doc"], "Caf serves  tea."; got != want {
		t.Errorf("non-ASCII doc = %q, want %q", got, want)
	}
	// pgraph/symbols.go:108-112: a spec of a multi-spec declaration takes
	// only its own doc.
	if n := overlayNode(t, src, id("const", "A")); n.Attrs["doc"] != "" {
		t.Errorf("A took the group doc: %q", n.Attrs["doc"])
	} else if _, ok := n.Attrs["doc"]; ok {
		t.Errorf("A carries an empty doc attr")
	}
	if got, want := overlayNode(t, src, id("const", "B")).Attrs["doc"], "B has its own doc."; got != want {
		t.Errorf("B doc = %q, want %q", got, want)
	}
	// pgraph/symbols.go:111-112: one spec takes the declaration's doc.
	if got, want := overlayNode(t, src, id("type", "Single")).Attrs["doc"], "Single is documented by its declaration."; got != want {
		t.Errorf("Single doc = %q, want %q", got, want)
	}
	// pgraph/symbols.go:200-202's twin: no doc, no attr.
	if _, ok := overlayNode(t, src, id("func", "Bare")).Attrs["doc"]; ok {
		t.Errorf("Bare carries a doc attr")
	}
}

// TestOverlay_SymCallsNames: the calls attr on funcs and methods only.
func TestOverlay_SymCallsNames(t *testing.T) {
	src := `package p

type T struct{}

var V = helper()

func Gen[X any](x X) {}

func (t T) M() { t.other() }

func A(s []int) {
	foo()
	x.Bar()
	Gen[int](1)
	Pair[int, string](2)
	(paren)()
	func() { inner() }()
	_ = len(s)
	s = append(s, 1)
	panic("x")
	foo()
}

func None() { _ = 1 }
`
	id := func(kind, q string) graph.ID { return graph.SymID(Lang, "p", kind, q) }
	if got, want := overlayNode(t, src, id("func", "A")).Attrs["calls"], "Bar,Gen,Pair,foo,inner,paren"; got != want {
		t.Errorf("A calls = %q, want %q", got, want)
	}
	if got, want := overlayNode(t, src, id("method", "T.M")).Attrs["calls"], "other"; got != want {
		t.Errorf("T.M calls = %q, want %q", got, want)
	}
	if _, ok := overlayNode(t, src, id("func", "None")).Attrs["calls"]; ok {
		t.Errorf("None carries a calls attr")
	}
	for _, kind := range []string{"var:V", "type:T"} {
		k, q, _ := strings.Cut(kind, ":")
		if _, ok := overlayNode(t, src, id(k, q)).Attrs["calls"]; ok {
			t.Errorf("%s carries a calls attr", kind)
		}
	}
	// The cap: 40 names, the first MaxOverlayCalls sorted, "+8" for the rest.
	var body, want []string
	for i := 0; i < MaxOverlayCalls+8; i++ {
		name := fmt.Sprintf("f%02d", i)
		body = append(body, name+"()")
		if i < MaxOverlayCalls {
			want = append(want, name)
		}
	}
	want = append(want, "+8")
	capped := "package p\nfunc Big() {\n" + strings.Join(body, "\n") + "\n}\n"
	if got := overlayNode(t, capped, id("func", "Big")).Attrs["calls"]; got != strings.Join(want, ",") {
		t.Errorf("capped calls = %q, want %q", got, strings.Join(want, ","))
	}
}

// TestOverlay_NewFileIsContainedByItsUnit: an overlay of a path the index
// lacks carries the Contains edge from unit:go:<dir> to the file.
func TestOverlay_NewFileIsContainedByItsUnit(t *testing.T) {
	_, edges := split(overlayOf(t, "newpkg/sub/x.go", "package sub\nfunc F() {}\n", 4))
	e, ok := edges[key("unit:go:newpkg/sub", graph.Contains, graph.FileID("newpkg/sub/x.go"))]
	if !ok {
		t.Fatalf("no Contains edge from unit:go:newpkg/sub; edges %v", edges)
	}
	if e.Tier != graph.Algorithmic || e.Conf != overlayConf || e.Prov.Extractor != overlayProv.Extractor || e.Prov.Version != overlayProv.Version || e.Rev != graph.RevOverlay("C1", 4) {
		t.Errorf("Contains edge = %+v, want the overlay's tier, conf, prov and rev", e)
	}
	if _, edges := split(overlayOf(t, "top.go", "package top\n", 1)); len(edges) != 1 {
		t.Errorf("a root file's edges = %v, want its Contains from unit:go:.", edges)
	} else if _, ok := edges[key("unit:go:.", graph.Contains, "file:top.go")]; !ok {
		t.Errorf("no Contains edge from unit:go:. in %v", edges)
	}
}

// TestOverlay_ExistingFileContainsKeyEqualsIndex: for a shapes file the
// overlay's Contains edge has the key the index writes.
func TestOverlay_ExistingFileContainsKeyEqualsIndex(t *testing.T) {
	root := shapes(t)
	snap, err := platform.ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"shape/shape.go", "app/app.go"} {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		contains := func(recs []graph.Record) []string {
			var keys []string
			for _, r := range recs {
				if r.Edge != nil && r.Edge.Pred == graph.Contains {
					keys = append(keys, r.Edge.Key())
				}
			}
			return keys
		}
		want := contains(newMapper(snap, []string{rel}, graph.RevCommit("C1")).records())
		got := contains(overlayOf(t, rel, string(src), 1))
		if len(want) != 1 || len(got) != 1 || got[0] != want[0] {
			t.Errorf("%s: overlay Contains keys %q, index %q", rel, got, want)
		}
	}
}
