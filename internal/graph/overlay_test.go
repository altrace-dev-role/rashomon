package graph

import "testing"

// TestOverlay_ReaderMergesCommitAndOverlaysBySeq: a reader at {C1, n}
// sees C1 and every overlay through n: a node's overlay version replaces
// its commit version from its seq on, a retraction at an overlay hides an
// edge from that seq on, the commit view never changes, and a reader at a
// later commit sees none of C1's overlays.
func TestOverlay_ReaderMergesCommitAndOverlaysBySeq(t *testing.T) {
	s := NewStore()
	c1 := RevCommit("C1")
	f := FileID("a.py")
	fn := SymID("py", "u", "func", "a.f")
	g := SymID("py", "u", "func", "a.g")
	s.AssertNode(Node{ID: fn, Kind: "sym", Attrs: map[string]string{"sig": "(x)"}, Tier: Algorithmic, Conf: 1, Rev: c1})
	s.AssertNode(Node{ID: g, Kind: "sym", Tier: Algorithmic, Conf: 1, Rev: c1})
	s.AssertEdge(Edge{Src: f, Pred: Declares, Dst: fn, Tier: Algorithmic, Conf: 1, Rev: c1})
	s.AssertEdge(Edge{Src: f, Pred: Declares, Dst: g, Tier: Algorithmic, Conf: 1, Rev: c1})
	s.AssertEdge(Edge{Src: fn, Pred: Calls, Dst: g, Tier: Algorithmic, Conf: 0.9, Rev: c1})

	ov := s.ApplyOverlay(f, []byte("def f(x, y): pass"), []Record{
		{Node: &Node{ID: fn, Kind: "sym", Attrs: map[string]string{"sig": "(x, y)"}, Tier: Algorithmic, Conf: 0.9}},
		{Edge: &Edge{Src: f, Pred: Declares, Dst: fn, Tier: Algorithmic, Conf: 0.9}},
	}, RevOverlay("C1", 12), Carried{Declares, Calls})
	if ov.State != OverlayLive || ov.Seq != 12 || ov.File != f || ov.Records != 2 {
		t.Fatalf("overlay = %+v", ov)
	}
	if n, _ := s.Node(fn, c1); n.Attrs["sig"] != "(x)" {
		t.Fatalf("commit view changed: %+v", n)
	}
	if n, _ := s.Node(fn, RevOverlay("C1", 11)); n.Attrs["sig"] != "(x)" {
		t.Fatalf("reader before the overlay's seq saw it: %+v", n)
	}
	n, ok := s.Node(fn, RevOverlay("C1", 12))
	if !ok || n.Attrs["sig"] != "(x, y)" || n.Rev.Kind != RevKindOverlay || n.Rev.Seq != 12 {
		t.Fatalf("reader at ov12 = %+v ok=%v", n, ok)
	}
	if _, ok := s.Node(g, RevOverlay("C1", 12)); ok {
		t.Fatal("g was not declared by the edit and must be retracted at ov12")
	}
	if _, ok := s.Node(g, c1); !ok {
		t.Fatal("g's retraction leaked into the commit view")
	}
	if es := s.Out(fn, RevOverlay("C1", 12), Calls); len(es) != 0 {
		t.Fatalf("f's call to g survived the overlay that dropped it: %+v", es)
	}
	if es := s.Out(fn, c1, Calls); len(es) != 1 {
		t.Fatalf("commit view lost f's call: %+v", es)
	}
	if d := s.Out(f, RevOverlay("C1", 16), Declares); len(d) != 1 || d[0].Dst != fn || d[0].Certain {
		t.Fatalf("declares at ov16 = %+v; an overlay edge is never certain", d)
	}
	if _, ok := s.Node(fn, RevCommit("C2")); ok {
		t.Fatal("a reader at C2 saw C1 records")
	}

	s.Reconcile("C2", map[ID]string{f: ContentDigest([]byte("def f(x, y): pass"))})
	if got := s.Overlays()[0].State; got != OverlaySubsumed {
		t.Fatalf("overlay after a matching index = %s, want subsumed", got)
	}
	s2 := NewStore()
	s2.ApplyOverlay(f, []byte("x"), nil, RevOverlay("C1", 3), Carried{Declares})
	s2.Reconcile("C2", map[ID]string{f: ContentDigest([]byte("y"))})
	if got := s2.Overlays()[0].State; got != OverlayStale {
		t.Fatalf("overlay after a differing index = %s, want stale", got)
	}
}

// TestOverlay_RevStringIsWhatALineCarries: "C1" and "C1+ov16".
func TestOverlay_RevStringIsWhatALineCarries(t *testing.T) {
	if got := RevCommit("C1").String(); got != "C1" {
		t.Fatal(got)
	}
	if got := RevOverlay("C1", 16).String(); got != "C1+ov16" {
		t.Fatal(got)
	}
}

// TestApplyOverlay_KeptSymbolKeepsUncarriedEdges: an overlay's silence on a
// predicate its extractor does not carry is unknown, not absent. Base rev:
// sym A has Calls (A->B), MayCall, References and ResolvesTo out-edges, and
// the file also declares C. The overlay re-declares A and drops C.
func TestApplyOverlay_KeptSymbolKeepsUncarriedEdges(t *testing.T) {
	c1 := RevCommit("C1")
	f := FileID("a.go")
	a := SymID("go", "u", "func", "A")
	b := SymID("go", "u", "func", "B")
	c := SymID("go", "u", "func", "C")
	x := SymID("go", "u", "func", "X")
	build := func() *Store {
		s := NewStore()
		for _, n := range []ID{a, b, c, x} {
			s.AssertNode(Node{ID: n, Kind: "sym", Tier: Algorithmic, Conf: 1, Rev: c1})
		}
		s.AssertNode(Node{ID: f, Kind: "file", Tier: Algorithmic, Conf: 1, Rev: c1})
		for _, d := range []ID{a, c} {
			s.AssertEdge(Edge{Src: f, Pred: Declares, Dst: d, Tier: Algorithmic, Conf: 1, Rev: c1})
		}
		s.AssertEdge(Edge{Src: f, Pred: Imports, Dst: x, Tier: Algorithmic, Conf: 1, Rev: c1})
		for _, p := range []Pred{Calls, MayCall, References, ResolvesTo} {
			s.AssertEdge(Edge{Src: a, Pred: p, Dst: b, Tier: Algorithmic, Conf: 1, Rev: c1})
		}
		s.AssertEdge(Edge{Src: c, Pred: Calls, Dst: b, Tier: Algorithmic, Conf: 1, Rev: c1})
		return s
	}
	recs := []Record{
		{Node: &Node{ID: f, Kind: "file", Tier: Algorithmic, Conf: 0.8}},
		{Node: &Node{ID: a, Kind: "sym", Tier: Algorithmic, Conf: 0.8}},
		{Edge: &Edge{Src: f, Pred: Declares, Dst: a, Tier: Algorithmic, Conf: 0.8}},
	}
	rev := RevOverlay("C1", 1)
	standing := func(s *Store, p Pred) bool { return len(s.Out(a, rev, p)) == 1 }

	s := build()
	s.ApplyOverlay(f, []byte("src"), recs, rev, Carried{Declares})
	for _, p := range []Pred{Calls, MayCall, References, ResolvesTo} {
		if !standing(s, p) {
			t.Errorf("(i) A's %s edge was retracted by an overlay whose extractor does not carry it", p)
		}
	}
	if es := s.Out(f, rev, Imports); len(es) != 1 {
		t.Errorf("(i) the file node's uncarried imports edge was retracted: %+v", es)
	}
	if _, ok := s.Node(c, rev); ok {
		t.Error("(i) C was dropped by the edit and must be retracted")
	}
	if es := s.Out(c, rev); len(es) != 0 {
		t.Errorf("(i) C's out-edges must go with it: %+v", es)
	}

	s = build()
	s.ApplyOverlay(f, []byte("src"), recs, rev, Carried{Declares, Calls})
	if standing(s, Calls) {
		t.Error("(ii) A->B Calls is carried and not produced: it must be retracted")
	}
	for _, p := range []Pred{MayCall, References, ResolvesTo} {
		if !standing(s, p) {
			t.Errorf("(ii) A's %s edge must stand", p)
		}
	}
	if es := s.Out(c, rev); len(es) != 0 {
		t.Errorf("(ii) C's out-edges must go with it: %+v", es)
	}
}
