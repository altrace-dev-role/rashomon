package engine

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// crossingPreds are the edges that cross a boundary: a call, a possible
// call, an import and a reference. ResolvesTo and the structural edges do
// not.
var crossingPreds = []graph.Pred{graph.Calls, graph.MayCall, graph.Imports, graph.References}

// boundaryConflicts is one boundary_cross for each edge the diff adds with
// a crossing predicate from a unit the rule's From names into a unit its To
// names; an edge that only changed is not added (Diff), so a crossing that
// stands at the base never fires. The conflict is in the source's unit and
// about the edge's source, a symbol or a file.
func boundaryConflicts(g Reader, d Diff, r Rule) []Conflict {
	var out []Conflict
	for _, rec := range d.Decls {
		e := rec.Edge
		if e == nil || !slices.Contains(crossingPreds, e.Pred) {
			continue
		}
		from, to := unitAt(g, d, e.Src), unitAt(g, d, e.Dst)
		if !underAny(from, r.From) || !underAny(to, r.To) {
			continue
		}
		out = append(out, Conflict{Kind: ConflictBoundaryCross, Edge: *e, Rev: d.Rev, Unit: from, RuleID: r.ID,
			Subject: e.Src, Evidence: []graph.ID{e.Src, e.Dst}})
	}
	return out
}

// underAny is whether unit is one of the unit ids given or under one by
// path segment: never a regexp (RES-2), never a bare string prefix, so
// internal/graph names internal/graph/engine and not internal/graphx.
func underAny(unit graph.ID, units []string) bool {
	u := string(unit)
	if u == "" {
		return false
	}
	for _, p := range units {
		if u == p || strings.HasPrefix(u, p+"/") {
			return true
		}
	}
	return false
}
