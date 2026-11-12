package engine

import (
	"maps"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Rule is a compiled rule as the conflict detectors read it: a spec.Rule of
// the knowledge graph's kinds with AppliesTo resolved to graph ids. Kind is
// a spec rule kind; From and To are unit-id prefixes; Predicate is a
// decision's check (paths_untouched, symbol_present); Source is the
// knowledge row id the rule was compiled from.
type Rule struct {
	ID        string
	Kind      string
	From      []string
	To        []string
	Predicate string
	AppliesTo []graph.ID
	Source    string
}

// Diff is what changed for a set of files between Base and Rev: Files are
// the files' ids and Touched the units that contain them; Decls are the
// records present at Rev and absent at Base, Removed the reverse, both
// decided by identity alone (a node's id, an edge's key), so a record
// changed in place is in neither (P2 ruling 3: the index and an overlay
// spell one declaration differently). Base is the revision DiffOf read
// against, for the detectors that compare a record with its base version;
// a Diff built by hand leaves it zero, which admits no record, so they
// compare nothing.
type Diff struct {
	Rev     graph.Rev
	Base    graph.Rev
	Touched []graph.ID
	Files   []graph.ID
	Decls   []graph.Record
	Removed []graph.Record
}

// DiffOf is the diff of files between base and cur. A file's records are
// its outgoing edges and the symbols it declares with theirs, read at each
// revision and compared by identity. Files is the input sorted and unique;
// Touched is the units that contain the files through Contains at cur, or
// at base for a file gone at cur; Decls and Removed are sorted by
// identity.
func DiffOf(g Reader, base, cur graph.Rev, files []graph.ID) Diff {
	fs := slices.Compact(slices.Sorted(slices.Values(files)))
	at, was := recordsAt(g, cur, fs), recordsAt(g, base, fs)
	units := map[graph.ID]bool{}
	for _, f := range fs {
		in := g.In(f, cur, graph.Contains)
		if len(in) == 0 {
			in = g.In(f, base, graph.Contains)
		}
		for _, e := range in {
			units[e.Src] = true
		}
	}
	return Diff{Rev: cur, Base: base, Files: fs, Touched: slices.Sorted(maps.Keys(units)),
		Decls: onlyIn(at, was), Removed: onlyIn(was, at)}
}

// recordsAt is the files' records at rev keyed by identity: "n" and the id
// for a node, "e" and the key for an edge.
func recordsAt(g Reader, rev graph.Rev, files []graph.ID) map[string]graph.Record {
	out := map[string]graph.Record{}
	edge := func(e graph.Edge) { out["e\x00"+e.Key()] = graph.Record{Edge: &e} }
	for _, f := range files {
		for _, e := range g.Out(f, rev) {
			edge(e)
			if e.Pred != graph.Declares {
				continue
			}
			if n, ok := g.Node(e.Dst, rev); ok {
				out["n\x00"+string(n.ID)] = graph.Record{Node: &n}
				for _, se := range g.Out(n.ID, rev) {
					edge(se)
				}
			}
		}
	}
	return out
}

// onlyIn is a's records whose identity b lacks, in identity order.
func onlyIn(a, b map[string]graph.Record) []graph.Record {
	var out []graph.Record
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if _, ok := b[k]; !ok {
			out = append(out, a[k])
		}
	}
	return out
}
