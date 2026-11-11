package gopg

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

const overlaySrc = `package shape

import "fmt"

// Shape is a thing with an area.
type Shape interface{ Area() int }

type Square struct{ N int }

type Stack[T any] struct{ items []T }

const Max = 10

const (
	Min  = 1
	_    = 2
	Mid  = 5
)

var Default = Square{N: 1}

func (s *Square) Area() int { return s.N * s.N }

func (s Square) Name() string { return fmt.Sprint("sq") }

func (k *Stack[T]) Push(v T) { k.items = append(k.items, v) }

func Sum(ss []Shape, extra ...int) (t int) {
	secret := "BODYSENTINEL"
	_ = secret
	for _, s := range ss {
		t += s.Area()
	}
	return t
}
`

var overlayBase = graph.RevCommit("C1")

func overlayOf(t *testing.T, rel, src string, seq int64) []graph.Record {
	t.Helper()
	recs, err := Resolver{}.Overlay(rel, []byte(src), overlayBase, seq)
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	return recs
}

// TestGopgOverlay_DeclarationsOnly: one sym node per declaration (function,
// method, type, const, var), a Declares edge from the file node to each, a
// file node, and nothing else: no call edge, no local, no import, no body.
func TestGopgOverlay_DeclarationsOnly(t *testing.T) {
	recs := overlayOf(t, "shape/shape.go", overlaySrc, 7)
	nodes, edges := split(recs)
	file := graph.FileID("shape/shape.go")
	want := map[graph.ID]string{
		"sym:go:shape:type:Shape":         "Shape",
		"sym:go:shape:type:Square":        "Square",
		"sym:go:shape:type:Stack":         "Stack",
		"sym:go:shape:const:Max":          "Max",
		"sym:go:shape:const:Min":          "Min",
		"sym:go:shape:const:Mid":          "Mid",
		"sym:go:shape:var:Default":        "Default",
		"sym:go:shape:method:Square.Area": "Area",
		"sym:go:shape:method:Square.Name": "Name",
		"sym:go:shape:method:Stack.Push":  "Push",
		"sym:go:shape:func:Sum":           "Sum",
	}
	syms := 0
	for id, n := range nodes {
		if n.Kind != "sym" {
			continue
		}
		syms++
		name, ok := want[id]
		if !ok {
			t.Errorf("unexpected sym %s", id)
			continue
		}
		if n.Attrs["name"] != name || n.Attrs["symkind"] == "" {
			t.Errorf("%s attrs = %v, want name %q and a symkind", id, n.Attrs, name)
		}
		if _, ok := edges[key(file, graph.Declares, id)]; !ok {
			t.Errorf("no Declares edge from %s to %s", file, id)
		}
	}
	if syms != len(want) {
		t.Errorf("%d syms, want %d", syms, len(want))
	}
	if len(edges) != len(want)+1 {
		t.Errorf("%d edges, want the %d Declares and the unit's one Contains", len(edges), len(want))
	}
	for _, e := range edges {
		isDeclares := e.Pred == graph.Declares && e.Src == file
		isContains := e.Pred == graph.Contains && e.Src == "unit:go:shape" && e.Dst == file
		if !isDeclares && !isContains {
			t.Errorf("edge %s %s %s: overlay edges are Declares from the file and one Contains from the unit", e.Src, e.Pred, e.Dst)
		}
	}
	if n := nodes["sym:go:shape:func:Sum"]; n.Attrs["sig"] != "func(ss []Shape, extra ...int) (t int)" {
		t.Errorf("Sum sig = %q", n.Attrs["sig"])
	}
	if n := nodes["sym:go:shape:method:Square.Area"]; n.Attrs["sig"] != "func() int" {
		t.Errorf("Area sig = %q", n.Attrs["sig"])
	}
	for _, n := range nodes {
		for k, v := range n.Attrs {
			if strings.Contains(v, "BODYSENTINEL") {
				t.Errorf("%s attr %s holds a function body", n.ID, k)
			}
		}
	}
}

// TestGopgOverlay_TierConfCertain: every record is ALGORITHMIC at 0.8, no
// edge is certain, and every record sits at the overlay revision.
func TestGopgOverlay_TierConfCertain(t *testing.T) {
	recs := overlayOf(t, "shape/shape.go", overlaySrc, 7)
	if len(recs) == 0 {
		t.Fatal("no records")
	}
	rev := graph.Rev{Kind: graph.RevKindOverlay, Commit: "C1", Seq: 7}
	for _, r := range recs {
		switch {
		case r.Node != nil:
			n := r.Node
			if n.Rev != rev {
				t.Errorf("node %s rev %v, want %v", n.ID, n.Rev, rev)
			}
			if n.Kind == "sym" && (n.Tier != graph.Algorithmic || n.Conf != 0.8) {
				t.Errorf("sym %s tier %s conf %v, want ALGORITHMIC at 0.8", n.ID, n.Tier, n.Conf)
			}
		case r.Edge != nil:
			e := r.Edge
			if e.Rev != rev || e.Tier != graph.Algorithmic || e.Conf != 0.8 || e.Certain {
				t.Errorf("edge %s: rev %v tier %s conf %v certain %v", e.Dst, e.Rev, e.Tier, e.Conf, e.Certain)
			}
		}
	}
}

// TestGopgOverlay_UnparsedYieldsNothing: content that does not parse gives
// no records and no error: the overlay says less, never wrong.
func TestGopgOverlay_UnparsedYieldsNothing(t *testing.T) {
	for _, src := range []string{"func (", "package p\nfunc F( {", "", "not go at all"} {
		recs, err := Resolver{}.Overlay("p/p.go", []byte(src), overlayBase, 1)
		if recs != nil || err != nil {
			t.Errorf("Overlay(%q) = %v, %v; want nil, nil", src, recs, err)
		}
	}
}

// TestGopgOverlay_DigestIsTheContents: the file node's digest attr is the
// content's ContentDigest, so a one-byte change is a different digest and
// the node carries no content.
func TestGopgOverlay_DigestIsTheContents(t *testing.T) {
	a := overlayOf(t, "shape/shape.go", overlaySrc, 1)
	b := overlayOf(t, "shape/shape.go", overlaySrc+"\n", 2)
	fileNode := func(recs []graph.Record) graph.Node {
		nodes, _ := split(recs)
		n, ok := nodes[graph.FileID("shape/shape.go")]
		if !ok || n.Kind != "file" {
			t.Fatalf("no file node in %d records", len(recs))
		}
		return n
	}
	if got, want := fileNode(a).Attrs["digest"], graph.ContentDigest([]byte(overlaySrc)); got != want {
		t.Errorf("digest = %q, want %q", got, want)
	}
	if fileNode(a).Attrs["digest"] == fileNode(b).Attrs["digest"] {
		t.Errorf("one extra byte left the digest unchanged")
	}
	for k, v := range fileNode(a).Attrs {
		if len(v) > 200 {
			t.Errorf("file attr %s is %d bytes: a node carries no content", k, len(v))
		}
	}
}

// TestGopgOverlay_SymbolIDsEqualM1: for the fixture's own source, the
// overlay's symbol ids are exactly the ids the M1 resolver gives the symbols
// pgraph placed in that file, so an overlay replaces a symbol and never
// duplicates it. The M1 side is the mapper over the real sync.
func TestGopgOverlay_SymbolIDsEqualM1(t *testing.T) {
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
		m1 := map[graph.ID]bool{}
		_, edges := split(newMapper(snap, []string{rel}, graph.RevCommit("C1")).records())
		for _, e := range edges {
			if e.Pred == graph.Declares && e.Src == graph.FileID(rel) {
				m1[e.Dst] = true
			}
		}
		if len(m1) == 0 {
			t.Fatalf("%s: the M1 resolver declared nothing", rel)
		}
		got := map[graph.ID]bool{}
		for _, r := range overlayOf(t, rel, string(src), 1) {
			if r.Node != nil && r.Node.Kind == "sym" {
				got[r.Node.ID] = true
			}
		}
		for id := range m1 {
			if !got[id] {
				t.Errorf("%s: M1 id %s missing from the overlay", rel, id)
			}
		}
		for id := range got {
			if !m1[id] {
				t.Errorf("%s: overlay id %s is not an M1 id", rel, id)
			}
		}
	}
}

// TestGopgOverlay_StoreReplacesNotDuplicates: through the store, an overlay
// of an edited file leaves one live node per id, and a dropped declaration
// is retracted.
func TestGopgOverlay_StoreReplacesNotDuplicates(t *testing.T) {
	root := shapes(t)
	snap, err := platform.ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	s := graph.NewStore()
	for _, r := range newMapper(snap, nil, graph.RevCommit("C1")).records() {
		if r.Node != nil {
			s.AssertNode(*r.Node)
		}
		if r.Edge != nil {
			s.AssertEdge(*r.Edge)
		}
	}
	src, err := os.ReadFile(filepath.Join(root, "shape", "shape.go"))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(src), "const Max = 10", "", 1) + "\nfunc Extra() int { return 1 }\n"
	rev := graph.RevOverlay("C1", 3)
	s.ApplyOverlay(graph.FileID("shape/shape.go"), []byte(edited), overlayOf(t, "shape/shape.go", edited, 3), rev, Resolver{}.OverlayCarried())
	if _, ok := s.Node("sym:go:shape:func:Extra", rev); !ok {
		t.Errorf("the new declaration is absent at the overlay")
	}
	if _, ok := s.Node("sym:go:shape:func:Extra", graph.RevCommit("C1")); ok {
		t.Errorf("the new declaration leaked into the commit view")
	}
	if _, ok := s.Node("sym:go:shape:const:Max", rev); ok {
		t.Errorf("the dropped declaration is still live at the overlay")
	}
	n, ok := s.Node("sym:go:shape:func:Sum", rev)
	if !ok || n.Tier != graph.Algorithmic {
		t.Errorf("Sum at the overlay = %v, %v; want the overlay's version", n.Tier, ok)
	}
}

// BenchmarkGopgOverlay is the parse cost K7 spends: one file of about 2,000
// lines and 300 declarations, parsed and mapped once.
func BenchmarkGopgOverlay(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("package big\n\n")
	for i := 0; i < 150; i++ {
		n := strconv.Itoa(i)
		sb.WriteString("type T" + n + " struct{ A, B int }\n\n")
		sb.WriteString("func (t *T" + n + ") M() int {\n\tx := t.A\n\tfor i := 0; i < 10; i++ {\n\t\tx += i * t.B\n\t}\n\tif x > 3 {\n\t\treturn x\n\t}\n\treturn 0\n}\n\n")
	}
	src := []byte(sb.String())
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if recs, err := (Resolver{}).Overlay("big/big.go", src, overlayBase, 1); err != nil || len(recs) == 0 {
			b.Fatal("no records", err)
		}
	}
}

// TestOverlayCarried_EqualsEmittedPredicates: the predicates of the edges
// Overlay emits from the file node and from the symbols the file declares
// are exactly OverlayCarried, over a file whose bodies import and call. A
// carried predicate the overlay never emits would retract the index's
// edges of it; an emitted one it does not carry would leave stale index
// edges standing.
func TestOverlayCarried_EqualsEmittedPredicates(t *testing.T) {
	recs := overlayOf(t, "shape/shape.go", overlaySrc, 1)
	got, want := strings.Join(emittedPreds(recs, graph.FileID("shape/shape.go")), ","), Resolver{}.OverlayCarried().String()
	if got != want {
		t.Errorf("Overlay emits {%s} from the file and its symbols, OverlayCarried is {%s}", got, want)
	}
}

// emittedPreds is the sorted set of predicates of the edges in recs whose
// source is file or a symbol file declares.
func emittedPreds(recs []graph.Record, file graph.ID) []string {
	from := map[graph.ID]bool{file: true}
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Src == file && r.Edge.Pred == graph.Declares {
			from[r.Edge.Dst] = true
		}
	}
	set := map[string]bool{}
	for _, r := range recs {
		if r.Edge != nil && from[r.Edge.Src] {
			set[string(r.Edge.Pred)] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}
