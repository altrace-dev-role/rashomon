package engine

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// DuplicateMinStems is the fewest distinct shared stems that make an added
// callable a likely duplicate. PROVISIONAL; QA's replay over twenty merged
// diffs from this repository fits k, with the fit set and the false-positive
// set disjoint and named before the replay; the director rules k before
// T2's pre-registration.
const DuplicateMinStems = 3

// nodeLister is the optional listing of every node at a revision; the store
// has it, Reader does not.
type nodeLister interface {
	Nodes(at graph.Rev) []graph.Node
}

var _ nodeLister = (*graph.Store)(nil)

// duplicateConflicts flags each callable the diff adds (a func or method
// whose id is not also removed) that shares at least DuplicateMinStems
// distinct stems of name and doc with a func or method of another unit that
// something calls at d.Rev: one conflict per added callable, against the
// candidate with the most shared stems, ties to the smallest id. P2 ruling
// 7(b): a candidate the added callable calls is not one for it, by a Calls
// or MayCall edge at d.Rev or by its short name in the added record's
// calls attr (the overlay's witness, whose records carry no out-edges).
// The exclusion is per candidate, so the callable still fires against
// another; a same-named function elsewhere is excluded too, a miss on the
// safe side. Output is sorted by Subject. It lists candidates through
// nodeLister; a Reader without it (anything but the store) yields nil.
func duplicateConflicts(g Reader, d Diff) []Conflict {
	lister, ok := g.(nodeLister)
	if !ok {
		return nil
	}
	removed := removedIDs(d)
	added := map[graph.ID]graph.Node{}
	for _, r := range d.Decls {
		if n := r.Node; n != nil && isCallable(*n) && !removed[n.ID] {
			added[n.ID] = *n
		}
	}
	if len(added) == 0 {
		return nil
	}
	type cand struct {
		node  graph.Node
		unit  graph.ID
		stems map[string]bool
	}
	var cands []cand
	for _, n := range lister.Nodes(d.Rev) {
		if !isCallable(n) {
			continue
		}
		if _, isAdded := added[n.ID]; isAdded || len(g.In(n.ID, d.Rev, graph.Calls)) == 0 {
			continue
		}
		_, u := unitOf(g, d.Rev, n)
		cands = append(cands, cand{n, u, stemSet(n)})
	}
	ids := make([]graph.ID, 0, len(added))
	for id := range added {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var out []Conflict
	for _, id := range ids {
		an := added[id]
		_, au := unitOf(g, d.Rev, an)
		mine := stemSet(an)
		callees, names := callsOf(g, d.Rev, an)
		best, bestN := graph.ID(""), 0
		for _, c := range cands {
			if c.unit == au || callees[c.node.ID] || names[short(c.node.ID)] {
				continue
			}
			shared := 0
			for t := range mine {
				if c.stems[t] {
					shared++
				}
			}
			if shared >= DuplicateMinStems && (shared > bestN || shared == bestN && c.node.ID < best) {
				best, bestN = c.node.ID, shared
			}
		}
		if bestN > 0 {
			out = append(out, Conflict{Kind: ConflictDuplicateAbstraction, Unit: au, Subject: id,
				Evidence: []graph.ID{id, best}, Rev: d.Rev})
		}
	}
	return out
}

// callsOf is what callable n calls at rev: the targets of its Calls and
// MayCall edges, and the short names its calls attr lists, comma
// separated (a capped list's "+N" tail names nothing).
func callsOf(g Reader, rev graph.Rev, n graph.Node) (map[graph.ID]bool, map[string]bool) {
	ids, names := map[graph.ID]bool{}, map[string]bool{}
	for _, e := range g.Out(n.ID, rev, graph.Calls, graph.MayCall) {
		ids[e.Dst] = true
	}
	for _, s := range strings.Split(n.Attrs["calls"], ",") {
		if s != "" {
			names[s] = true
		}
	}
	return ids, names
}

// isCallable is a func or method symbol.
func isCallable(n graph.Node) bool {
	k := n.Attrs["symkind"]
	return k == "func" || k == "method"
}

// stemSet is the distinct stems of a callable's name and doc.
func stemSet(n graph.Node) map[string]bool {
	name := n.Attrs["name"]
	if name == "" {
		name = short(n.ID)
	}
	out := map[string]bool{}
	for _, t := range Tokens(name + " " + n.Attrs["doc"]) {
		out[t] = true
	}
	return out
}
