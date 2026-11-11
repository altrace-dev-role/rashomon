package engine

import (
	"reflect"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The conflict fixtures: a hand-built store in a neutral language tag, at
// commit C1 and at overlays on it, so each detector's case is the few
// records it reads and nothing else.
const fxLang = "lx"

var fxC1 = graph.RevCommit("C1")

func fxOv(seq int64) graph.Rev { return graph.RevOverlay("C1", seq) }

func fxUnit(path string) graph.ID { return graph.UnitID(fxLang, path) }

func fxSym(unit, kind, qualified string) graph.ID {
	return graph.SymID(fxLang, unit, kind, qualified)
}

type fx struct{ s *graph.Store }

func newFx() *fx { return &fx{s: graph.NewStore()} }

// file asserts a file the unit contains, at rev.
func (f *fx) file(unit, rel string, rev graph.Rev) graph.ID {
	id := graph.FileID(rel)
	f.s.AssertNode(graph.Node{ID: id, Kind: "file", Rev: rev})
	f.s.AssertEdge(graph.Edge{Src: fxUnit(unit), Pred: graph.Contains, Dst: id, Rev: rev})
	return id
}

// sym asserts a symbol the file declares, at rev, with its name and kind
// attrs and any others given.
func (f *fx) sym(file graph.ID, unit, kind, qualified string, rev graph.Rev, attrs map[string]string) graph.ID {
	id := fxSym(unit, kind, qualified)
	a := map[string]string{"name": qualified, "symkind": kind}
	for k, v := range attrs {
		a[k] = v
	}
	f.s.AssertNode(graph.Node{ID: id, Kind: "sym", Attrs: a, Rev: rev})
	f.s.AssertEdge(graph.Edge{Src: file, Pred: graph.Declares, Dst: id, Rev: rev})
	return id
}

func (f *fx) edge(src graph.ID, p graph.Pred, dst graph.ID, rev graph.Rev, attrs map[string]string) {
	f.s.AssertEdge(graph.Edge{Src: src, Pred: p, Dst: dst, Attrs: attrs, Rev: rev})
}

// drop retracts a symbol and its declaring edge at rev, as an edit that
// deletes the declaration does.
func (f *fx) drop(file, id graph.ID, rev graph.Rev) {
	f.s.RetractNode(id, rev)
	f.s.RetractEdge(file, graph.Declares, id, rev)
}

// labels is each record as "n <id>" or "e <src> <pred> <dst>", sorted, so
// a test compares sets and the order is TestConflicts_Deterministic's.
func labels(rs []graph.Record) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		switch {
		case r.Node != nil:
			out = append(out, "n "+string(r.Node.ID))
		case r.Edge != nil:
			out = append(out, "e "+string(r.Edge.Src)+" "+string(r.Edge.Pred)+" "+string(r.Edge.Dst))
		}
	}
	slices.Sort(out)
	return out
}

func sortedStrings(ss ...string) []string {
	out := slices.Clone(ss)
	slices.Sort(out)
	return out
}

// TestDiffOf_AddedAndRemovedDecls: an edit that drops B, adds C and
// rewrites A's doc. Decls holds the records present at cur and absent at
// base (C, its declaring edge, A's call to C); Removed the reverse (B, its
// declaring edge, A's call to B). Identity alone decides, a node's id and
// an edge's key (P2 ruling 3): A, changed in place, is in neither. The
// ruling's pin: the index's A against an overlay of the same content,
// spelled as the overlay spells it (another tier, conf, prov and attrs),
// is no change; the overlay adding B beside it adds B alone.
func TestDiffOf_AddedAndRemovedDecls(t *testing.T) {
	f := newFx()
	fl := f.file("a", "a/one.lx", fxC1)
	a := f.sym(fl, "a", "func", "A", fxC1, map[string]string{"doc": "old", "line": "3"})
	b := f.sym(fl, "a", "func", "B", fxC1, map[string]string{"line": "9"})
	f.edge(a, graph.Calls, b, fxC1, nil)

	ov := fxOv(1)
	f.drop(fl, b, ov)
	f.s.RetractEdge(a, graph.Calls, b, ov)
	c := f.sym(fl, "a", "func", "C", ov, map[string]string{"line": "9"})
	f.s.AssertNode(graph.Node{ID: a, Kind: "sym", Rev: ov,
		Attrs: map[string]string{"name": "A", "symkind": "func", "doc": "new", "line": "3"}})
	f.edge(a, graph.Calls, c, ov, nil)

	d := DiffOf(f.s, fxC1, ov, []graph.ID{fl})
	if d.Rev != ov || d.Base != fxC1 {
		t.Errorf("Rev, Base = %v, %v, want %v, %v", d.Rev, d.Base, ov, fxC1)
	}
	ar, cr, br, fs := string(a), string(c), string(b), string(fl)
	want := sortedStrings("n "+cr, "e "+fs+" declares "+cr, "e "+ar+" calls "+cr)
	if got := labels(d.Decls); !slices.Equal(got, want) {
		t.Errorf("Decls = %q\nwant %q", got, want)
	}
	want = sortedStrings("n "+br, "e "+fs+" declares "+br, "e "+ar+" calls "+br)
	if got := labels(d.Removed); !slices.Equal(got, want) {
		t.Errorf("Removed = %q\nwant %q", got, want)
	}
	if same := DiffOf(f.s, ov, ov, []graph.ID{fl}); len(same.Decls)+len(same.Removed) != 0 {
		t.Errorf("a rev against itself: Decls %q Removed %q, want none", labels(same.Decls), labels(same.Removed))
	}

	g := newFx()
	gf := g.file("a", "a/one.lx", fxC1)
	ga, gb := fxSym("a", "func", "A"), fxSym("a", "func", "B")
	g.s.AssertNode(graph.Node{ID: ga, Kind: "sym", Tier: graph.Proven, Conf: 1, Rev: fxC1, Prov: graph.Prov{Extractor: "lx-index"},
		Attrs: map[string]string{"name": "A", "symkind": "func", "doc": "A does a.", "line": "3"}})
	g.edge(gf, graph.Declares, ga, fxC1, nil)
	spell := func(id graph.ID, name string, rev graph.Rev) {
		g.s.AssertNode(graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: 0.8, Rev: rev, Prov: graph.Prov{Extractor: "lx-overlay"},
			Attrs: map[string]string{"name": name, "symkind": "func", "sig": "func()"}})
		g.s.AssertEdge(graph.Edge{Src: gf, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: 0.8, Rev: rev})
	}
	spell(ga, "A", fxOv(1))
	if d := DiffOf(g.s, fxC1, fxOv(1), []graph.ID{gf}); len(d.Decls)+len(d.Removed) != 0 {
		t.Errorf("the overlay's spelling of the index's A: Decls %q Removed %q, want none", labels(d.Decls), labels(d.Removed))
	}
	spell(ga, "A", fxOv(2))
	spell(gb, "B", fxOv(2))
	d = DiffOf(g.s, fxC1, fxOv(2), []graph.ID{gf})
	want = sortedStrings("n "+string(gb), "e "+string(gf)+" declares "+string(gb))
	if got := labels(d.Decls); !slices.Equal(got, want) || len(d.Removed) != 0 {
		t.Errorf("an overlay of A and B: Decls %q Removed %q, want Decls %q alone", got, labels(d.Removed), want)
	}
}

// TestDiffOf_ChangeInPlaceIsNoChange: a declaration that only moved (its
// line attr) is no change, nor is a call whose argc changed: the Go index
// puts a line on every symbol, so a compare by attrs would put every
// declaration below an edit in the diff. The control is a new edge
// between the same two symbols under another predicate: a new key.
func TestDiffOf_ChangeInPlaceIsNoChange(t *testing.T) {
	f := newFx()
	fl := f.file("a", "a/one.lx", fxC1)
	a := f.sym(fl, "a", "func", "A", fxC1, map[string]string{"line": "3", "doc": "same"})
	b := f.sym(fl, "a", "func", "B", fxC1, map[string]string{"line": "7"})
	f.edge(a, graph.Calls, b, fxC1, map[string]string{"line": "4", "argc": "1"})

	ov := fxOv(1)
	f.s.AssertNode(graph.Node{ID: a, Kind: "sym", Rev: ov,
		Attrs: map[string]string{"name": "A", "symkind": "func", "line": "13", "doc": "same"}})
	f.edge(a, graph.Calls, b, ov, map[string]string{"line": "14", "argc": "1"})

	d := DiffOf(f.s, fxC1, ov, []graph.ID{fl})
	if len(d.Decls)+len(d.Removed) != 0 {
		t.Fatalf("a line move: Decls %q Removed %q, want none", labels(d.Decls), labels(d.Removed))
	}
	f.edge(a, graph.Calls, b, fxOv(2), map[string]string{"line": "14", "argc": "2"})
	if d = DiffOf(f.s, fxC1, fxOv(2), []graph.ID{fl}); len(d.Decls)+len(d.Removed) != 0 {
		t.Fatalf("an argc change: Decls %q Removed %q, want none", labels(d.Decls), labels(d.Removed))
	}
	f.edge(a, graph.MayCall, b, fxOv(3), nil)
	d = DiffOf(f.s, fxC1, fxOv(3), []graph.ID{fl})
	e := "e " + string(a) + " " + string(graph.MayCall) + " " + string(b)
	if !slices.Equal(labels(d.Decls), []string{e}) || len(d.Removed) != 0 {
		t.Fatalf("a new edge: Decls %q Removed %q, want %q in Decls alone", labels(d.Decls), labels(d.Removed), e)
	}
}

// TestDiffOf_TouchedIsUnitsOfFiles: Touched is the units that contain the
// files, through Contains, at cur and, for a file gone at cur, at base;
// Files is the input sorted and unique; an untouched unit is absent.
func TestDiffOf_TouchedIsUnitsOfFiles(t *testing.T) {
	f := newFx()
	fa := f.file("a", "a/one.lx", fxC1)
	fb := f.file("b", "b/two.lx", fxC1)
	f.file("c", "c/three.lx", fxC1)
	fd := f.file("d", "d/four.lx", fxC1)
	ov := fxOv(1)
	f.s.RetractNode(fd, ov)
	f.s.RetractEdge(fxUnit("d"), graph.Contains, fd, ov)

	d := DiffOf(f.s, fxC1, ov, []graph.ID{fd, fb, fa, fb})
	if want := []graph.ID{fa, fb, fd}; !slices.Equal(d.Files, want) {
		t.Errorf("Files = %q, want %q", d.Files, want)
	}
	if want := []graph.ID{fxUnit("a"), fxUnit("b"), fxUnit("d")}; !slices.Equal(d.Touched, want) {
		t.Errorf("Touched = %q, want %q", d.Touched, want)
	}
	again := DiffOf(f.s, fxC1, ov, []graph.ID{fa, fb, fd})
	if !reflect.DeepEqual(d, again) {
		t.Errorf("DiffOf depends on the files' order:\n%+v\n%+v", d, again)
	}
}
