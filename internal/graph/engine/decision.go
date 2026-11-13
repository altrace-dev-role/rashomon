package engine

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// decisionConflicts is a decision rule's conflicts by its predicate; an
// unknown predicate raises none.
func decisionConflicts(g Reader, d Diff, r Rule) []Conflict {
	switch r.Predicate {
	case spec.PredicatePathsUntouched:
		return pathsTouched(g, d, r)
	case spec.PredicateSymbolPresent:
		return symbolsAbsent(g, d, r)
	}
	return nil
}

// pathsTouched is paths_untouched: one decision_contradicted per id the
// rule applies to that the diff touches, a file in Files or a unit in
// Touched, and one per file in Files that a pattern of the rule's Paths
// matches and AppliesTo did not already name.
func pathsTouched(g Reader, d Diff, r Rule) []Conflict {
	var out []Conflict
	for _, id := range r.AppliesTo {
		if slices.Contains(d.Files, id) || slices.Contains(d.Touched, id) {
			out = append(out, ruleConflict(g, d, r, ConflictDecisionContradicted, id))
		}
	}
	for _, id := range filesUnderPaths(d, r) {
		out = append(out, ruleConflict(g, d, r, ConflictDecisionContradicted, id))
	}
	return out
}

// symbolsAbsent is symbol_present: one decision_contradicted for each
// callable the diff adds in a touched unit (under one of the units the
// rule names, when it names any) that has no Calls, MayCall or References
// edge at d.Rev to a symbol the rule names; the evidence is the callable,
// then the symbols it misses. A rule naming no symbol raises none. P2
// ruling 1: a callable whose record at d.Rev is an overlay's raises none,
// since its out-edges are unknown, not absent (the Go overlay carries
// Declares alone), so the predicate fires at commit revisions only.
func symbolsAbsent(g Reader, d Diff, r Rule) []Conflict {
	var need []graph.ID
	var scope []string
	for _, id := range r.AppliesTo {
		switch {
		case strings.HasPrefix(string(id), "sym:"):
			need = append(need, id)
		case strings.HasPrefix(string(id), "unit:"):
			scope = append(scope, string(id))
		}
	}
	if len(need) == 0 {
		return nil
	}
	need = slices.Compact(slices.Sorted(slices.Values(need)))
	removed := removedIDs(d)
	var out []Conflict
	for _, rec := range d.Decls {
		n := rec.Node
		if n == nil || !isCallable(*n) || removed[n.ID] {
			continue
		}
		u := unitAt(g, d, n.ID)
		if !slices.Contains(d.Touched, u) || len(scope) > 0 && !underAny(u, scope) {
			continue
		}
		if cur, ok := g.Node(n.ID, d.Rev); !ok || cur.Rev.Kind == graph.RevKindOverlay {
			continue
		}
		reached := map[graph.ID]bool{}
		for _, e := range g.Out(n.ID, d.Rev, graph.Calls, graph.MayCall, graph.References) {
			reached[e.Dst] = true
		}
		ev := []graph.ID{n.ID}
		for _, s := range need {
			if !reached[s] {
				ev = append(ev, s)
			}
		}
		if len(ev) > 1 {
			out = append(out, Conflict{Kind: ConflictDecisionContradicted, Rev: d.Rev, Unit: u, RuleID: r.ID,
				Subject: n.ID, Evidence: ev})
		}
	}
	return out
}
