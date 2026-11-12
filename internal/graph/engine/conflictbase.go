package engine

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// The post's conflict check (W-M2.4 hook seam, card m2-seam1 D2, D3). The
// post never loads the index: the prompt, where the index is loaded, keeps
// the base records the detectors read for an edit under the rules, and the
// post applies the edit's overlay group over them and calls DiffOf and
// Conflicts unchanged. What the slice holds follows from what they read of
// one edited file:
//   - DiffOf compares the file's out-edges, its declared symbols and theirs
//     by identity, at the base and at the overlay. A base record the slice
//     lacks reads as added at the overlay, and of the live detectors only
//     boundaryConflicts reads added records, and only edges of a crossing
//     predicate from a unit a rule's From names into a unit its To names. So
//     the slice holds each such base edge and what DiffOf and
//     ApplyOverlayDigest walk to reach it: the Declares edge into a symbol
//     source and that symbol's node, and the Contains edges into each end.
//     ApplyOverlayDigest then retracts a slice edge exactly when it retracts
//     it in the whole index, so an edge the base already held is never added.
//   - paths_untouched and protect_path read the diff's Files, which is the
//     input, and Touched, which the overlay's own Contains edge gives: no
//     base record. The base keeps the content digest of each file they
//     reach, so an edit that restores a file to its base content passes no
//     file: the tree no longer shows the conflict.
//   - symbol_present is silent at an overlay revision (P2 ruling 1),
//     signature_mismatch needs an arity the Go index does not hold, and
//     duplicate_abstraction is withheld; none reads a base record here.
// TestConflictBase_SliceEqualsFullLoad pins the slice against the whole
// index over a case of each live kind, and
// TestConflictBase_EveryLiveKindHasACase fails when a kind joins the live
// set without one.

// ConflictBase is the slice of an index for a rule set: Records, at the
// index's commit, and Digests, the content digest at the commit of each file
// a paths_untouched or protect_path rule reaches (an AppliesTo file, a file
// of an AppliesTo unit, an index file a pattern of Paths matches).
type ConflictBase struct {
	Records []graph.Record
	Digests map[graph.ID]string
}

// ConflictBaseOf is the slice of g at rev for rules. Cost: one pass over the
// edges at rev when a boundary rule is present, one over the nodes when a
// path rule names a pattern, and nothing else.
func ConflictBaseOf(g *graph.Store, rev graph.Rev, rules []Rule) ConflictBase {
	var bounds, paths []Rule
	for _, r := range rules {
		switch {
		case spec.RuleKind(r.Kind) == spec.Boundary:
			bounds = append(bounds, r)
		case pathRule(r):
			paths = append(paths, r)
		}
	}
	b := ConflictBase{Digests: map[graph.ID]string{}}
	seen := map[string]bool{}
	add := func(rec graph.Record) {
		if k := recordKey(rec); !seen[k] {
			seen[k] = true
			b.Records = append(b.Records, rec)
		}
	}
	if len(bounds) > 0 {
		for _, e := range g.Edges(rev) {
			if !slices.Contains(crossingPreds, e.Pred) || !crossesAny(g, rev, e, bounds) {
				continue
			}
			add(graph.Record{Edge: &e})
			for _, end := range []graph.ID{e.Src, e.Dst} {
				for _, c := range g.In(end, rev, graph.Contains) {
					add(graph.Record{Edge: &c})
				}
			}
			if n, ok := g.Node(e.Src, rev); ok && strings.HasPrefix(string(e.Src), "sym:") {
				add(graph.Record{Node: &n})
				for _, d := range g.In(e.Src, rev, graph.Declares) {
					add(graph.Record{Edge: &d})
				}
			}
		}
	}
	for _, f := range reachedFiles(g, rev, paths) {
		if n, ok := g.Node(f, rev); ok && n.Attrs["digest"] != "" {
			b.Digests[f] = n.Attrs["digest"]
		}
	}
	return b
}

// Store is a store holding b's records at commit.
func (b ConflictBase) Store(commit string) *graph.Store {
	s := graph.NewStore()
	rev := graph.RevCommit(commit)
	for _, r := range b.Records {
		if r.Node != nil {
			n := *r.Node
			n.Rev = rev
			s.AssertNode(n)
		}
		if r.Edge != nil {
			e := *r.Edge
			e.Rev = rev
			s.AssertEdge(e)
		}
	}
	return s
}

// InScope is whether an edit of file, in unit, can raise a conflict under
// rules: its unit under a boundary rule's From, or the file reached by a
// paths_untouched or protect_path rule (in Digests, itself or its unit in
// AppliesTo, or a pattern of Paths matching it, which covers a file the
// index does not hold yet). An edit out of scope raises nothing, so the post
// checks and records nothing for it (card D4).
func (b ConflictBase) InScope(rules []Rule, file, unit graph.ID) bool {
	if _, ok := b.Digests[file]; ok {
		return true
	}
	rel := strings.TrimPrefix(string(file), "file:")
	for _, r := range rules {
		switch {
		case spec.RuleKind(r.Kind) == spec.Boundary:
			if underAny(unit, r.From) {
				return true
			}
		case pathRule(r):
			if slices.Contains(r.AppliesTo, unit) || slices.Contains(r.AppliesTo, file) ||
				slices.ContainsFunc(r.Paths, func(p string) bool { return spec.MatchPath(p, rel) }) {
				return true
			}
		}
	}
	return false
}

// EditConflicts is the conflicts an edit raises: g holds the base at the
// group's commit (the slice, or in the differential pin the whole index),
// og is the edit's landed overlay group. The group is applied to g at its
// own revision, then Conflicts under rules over DiffOf of the group's file
// between the commit and that revision, as an index-time check calls them.
// baseDigest is the file's content digest at the commit, "" for none: a
// group at that digest restores the file and raises nothing.
func EditConflicts(g *graph.Store, rules []Rule, og graph.OverlayGroup, baseDigest string) []Conflict {
	if baseDigest != "" && baseDigest == og.Digest {
		return nil
	}
	cur := graph.RevOverlay(og.Commit, og.Seq)
	g.ApplyOverlayDigest(og.File, og.Digest, og.Recs, cur, og.Carried)
	return Conflicts(g, DiffOf(g, graph.RevCommit(og.Commit), cur, []graph.ID{og.File}), rules)
}

// pathRule is whether r reads the diff's files: a paths_untouched decision
// or a protect_path rule.
func pathRule(r Rule) bool {
	k := spec.RuleKind(r.Kind)
	return k == spec.ProtectPath || k == spec.Decision && r.Predicate == spec.PredicatePathsUntouched
}

// crossesAny is whether e runs from a unit under a rule's From into a unit
// under that rule's To, at rev.
func crossesAny(g Reader, rev graph.Rev, e graph.Edge, rules []Rule) bool {
	from, to := unitIDOf(g, rev, e.Src), unitIDOf(g, rev, e.Dst)
	return slices.ContainsFunc(rules, func(r Rule) bool { return underAny(from, r.From) && underAny(to, r.To) })
}

// reachedFiles is the files rules reach at rev, each once, in rule order.
func reachedFiles(g *graph.Store, rev graph.Rev, rules []Rule) []graph.ID {
	var out []graph.ID
	seen := map[graph.ID]bool{}
	add := func(id graph.ID) {
		if strings.HasPrefix(string(id), "file:") && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	var nodes []graph.Node
	for _, r := range rules {
		for _, id := range r.AppliesTo {
			add(id)
			if strings.HasPrefix(string(id), "unit:") {
				for _, e := range g.Out(id, rev, graph.Contains) {
					add(e.Dst)
				}
			}
		}
		if len(r.Paths) == 0 {
			continue
		}
		if nodes == nil {
			nodes = g.Nodes(rev)
		}
		for _, n := range nodes {
			if rel, ok := strings.CutPrefix(string(n.ID), "file:"); ok &&
				slices.ContainsFunc(r.Paths, func(p string) bool { return spec.MatchPath(p, rel) }) {
				add(n.ID)
			}
		}
	}
	return out
}

// recordKey is rec's identity, as recordsAt keys it.
func recordKey(rec graph.Record) string {
	if rec.Node != nil {
		return "n\x00" + string(rec.Node.ID)
	}
	return "e\x00" + rec.Edge.Key()
}
