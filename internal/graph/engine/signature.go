package engine

import "github.com/altrace-dev-role/rashomon/internal/graph"

// signatureConflicts is signature_mismatch: for each callable a file of
// the diff declares at d.Rev whose sig there differs from its sig at
// d.Base, each arity conflict on it at d.Rev, kept whole (Edge, Callee,
// Want, Got) and placed in the callable's unit, with the callable and the
// caller as evidence. Both sigs must be present: a sig appearing or going
// is a change of spelling, not of signature (the Go index writes none).
// The callable is changed in place, so it is in neither Decls nor Removed;
// the detector reads it at both revisions. A Diff with no Base compares
// nothing.
func signatureConflicts(g Reader, d Diff) []Conflict {
	if d.Base == (graph.Rev{}) {
		return nil
	}
	seen := map[graph.ID]bool{}
	var out []Conflict
	for _, f := range d.Files {
		for _, e := range g.Out(f, d.Rev, graph.Declares) {
			id := e.Dst
			if seen[id] {
				continue
			}
			seen[id] = true
			cur, ok := g.Node(id, d.Rev)
			base, okBase := g.Node(id, d.Base)
			if !ok || !okBase || cur.Attrs["sig"] == "" || base.Attrs["sig"] == "" || cur.Attrs["sig"] == base.Attrs["sig"] {
				continue
			}
			u := unitIDOf(g, d.Rev, id)
			for _, c := range ArityConflicts(g, id, d.Rev) {
				c.Kind, c.Unit, c.Subject, c.Evidence = ConflictSignatureMismatch, u, id, []graph.ID{id, c.Edge.Src}
				out = append(out, c)
			}
		}
	}
	return out
}
