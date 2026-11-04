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
	}, RevOverlay("C1", 12))
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
	s2.ApplyOverlay(f, []byte("x"), nil, RevOverlay("C1", 3))
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
