// Package engine is the task-aware decision engine's read side over the
// platform graph: scope, traversal and the conflict step. It names no
// language (TestEngine_NoLanguageLiterals); it reads records at a revision
// and every answer carries that revision.
package engine

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Reader is what the engine reads: the store, at a revision.
type Reader interface {
	Node(id graph.ID, at graph.Rev) (graph.Node, bool)
	Out(id graph.ID, at graph.Rev, preds ...graph.Pred) []graph.Edge
	In(id graph.ID, at graph.Rev, preds ...graph.Pred) []graph.Edge
}

// Neighborhood is the nodes reached from the seeds with their distance,
// and the revision they were read at.
type Neighborhood struct {
	Rev      graph.Rev
	Distance map[graph.ID]int
}

// IDs is the neighborhood's ids ordered by distance, then id.
func (n Neighborhood) IDs() []graph.ID {
	out := make([]graph.ID, 0, len(n.Distance))
	for id := range n.Distance {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		if n.Distance[out[i]] != n.Distance[out[j]] {
			return n.Distance[out[i]] < n.Distance[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// traversed is the predicates a traversal follows in both directions.
func traversed() []graph.Pred {
	return []graph.Pred{
		graph.Calls, graph.MayCall, graph.References, graph.Reads, graph.Writes,
		graph.Exposes, graph.Consumes, graph.ResolvesTo, graph.Imports, graph.Declares,
		graph.CoChangedWith, graph.DuplicatesValue, graph.Crosses,
	}
}

// Traverse is the breadth-first neighborhood of seeds to depth, over the
// traversed predicates in both directions, at rev.
func Traverse(g Reader, seeds []graph.ID, depth int, rev graph.Rev) Neighborhood {
	seen := map[graph.ID]int{}
	for _, s := range seeds {
		seen[s] = 0
	}
	frontier := seeds
	preds := traversed()
	for d := 1; d <= depth && len(frontier) > 0; d++ {
		var next []graph.ID
		for _, id := range frontier {
			for _, e := range g.Out(id, rev, preds...) {
				if _, ok := seen[e.Dst]; !ok {
					seen[e.Dst] = d
					next = append(next, e.Dst)
				}
			}
			for _, e := range g.In(id, rev, preds...) {
				if _, ok := seen[e.Src]; !ok {
					seen[e.Src] = d
					next = append(next, e.Src)
				}
			}
		}
		frontier = next
	}
	return Neighborhood{Rev: rev, Distance: seen}
}
