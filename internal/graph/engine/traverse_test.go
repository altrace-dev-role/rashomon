package engine

import (
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestTraverse_ShortestDistanceBothWaysToDepth (W-DEBT.6): a seed is at
// distance zero, a node reached twice keeps its shortest distance, edges
// are walked in both directions, and nothing past the depth is reached.
func TestTraverse_ShortestDistanceBothWaysToDepth(t *testing.T) {
	rev := graph.RevCommit("c1")
	s := graph.NewStore()
	const a, b, c, d = graph.ID("sym:py:.:func:m.a"), graph.ID("sym:py:.:func:m.b"), graph.ID("sym:py:.:func:m.c"), graph.ID("sym:py:.:func:m.d")
	for _, id := range []graph.ID{a, b, c, d} {
		s.AssertNode(graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev})
	}
	for _, e := range [][2]graph.ID{{a, b}, {b, c}, {a, c}, {c, d}} {
		s.AssertEdge(graph.Edge{Src: e[0], Pred: graph.Calls, Dst: e[1], Tier: graph.Algorithmic, Conf: 1, Rev: rev})
	}
	got := Traverse(s, []graph.ID{a}, 2, rev).Distance
	if want := map[graph.ID]int{a: 0, b: 1, c: 1, d: 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("from a at depth 2: %v; want %v (c once at its shortest, d past c)", got, want)
	}
	got = Traverse(s, []graph.ID{a}, 1, rev).Distance
	if want := map[graph.ID]int{a: 0, b: 1, c: 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("from a at depth 1: %v; want %v (nothing past the depth)", got, want)
	}
	got = Traverse(s, []graph.ID{d}, 1, rev).Distance
	if want := map[graph.ID]int{d: 0, c: 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("from d at depth 1: %v; want %v (the in-edge walked)", got, want)
	}
}
