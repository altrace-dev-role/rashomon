package engine

import (
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// ConflictKind is what a conflict is about.
type ConflictKind string

// The conflict kinds: arity, which ArityConflicts computes, and the five
// the knowledge and session graphs' rules and a diff raise (W-M2.2 computes
// them; W-M2.0 fixes their names).
const (
	ConflictArity                ConflictKind = "arity"
	ConflictBoundaryCross        ConflictKind = "boundary_cross"
	ConflictDecisionContradicted ConflictKind = "decision_contradicted"
	ConflictSignatureMismatch    ConflictKind = "signature_mismatch"
	ConflictDuplicateAbstraction ConflictKind = "duplicate_abstraction"
	ConflictProtectedPath        ConflictKind = "protected_path"
)

// ConflictKinds is the closed list: arity, then the five in the order above.
var ConflictKinds = []ConflictKind{ConflictArity, ConflictBoundaryCross, ConflictDecisionContradicted,
	ConflictSignatureMismatch, ConflictDuplicateAbstraction, ConflictProtectedPath}

// Conflict is one mismatch as the reader sees it at Rev. An arity conflict
// is between a caller and a callee: the caller passes Got arguments, the
// callee's signature at Rev admits Want. The other kinds name the Unit the
// conflict is in, the RuleID that raised it (empty when no rule did), the
// Subject it is about and the Evidence ids that show it.
type Conflict struct {
	Kind   ConflictKind
	Edge   graph.Edge
	Callee graph.Node
	Want   string
	Got    int
	Rev    graph.Rev

	Unit     graph.ID
	RuleID   string
	Subject  graph.ID
	Evidence []graph.ID
}

// Key is the conflict's identity across recomputations: the edge's.
func (c Conflict) Key() string { return c.Edge.Key() }

// ArityConflicts is every calls edge into callee at rev whose argument
// count the callee's signature at rev does not admit. The callee is read
// at rev, so an edit's overlay signature is the one compared, and a caller
// edited after it is compared against the new signature, not the
// committed one.
func ArityConflicts(g Reader, callee graph.ID, rev graph.Rev) []Conflict {
	n, ok := g.Node(callee, rev)
	if !ok {
		return nil
	}
	min, max, known := arity(n)
	if !known {
		return nil
	}
	var out []Conflict
	for _, e := range g.In(callee, rev, graph.Calls) {
		argc, err := strconv.Atoi(e.Attrs["argc"])
		if err != nil {
			continue
		}
		if argc < min || (max >= 0 && argc > max) {
			out = append(out, Conflict{Kind: ConflictArity, Edge: e, Callee: n, Want: n.Attrs["sig"], Got: argc, Rev: rev})
		}
	}
	return out
}

// CallerConflicts is every arity conflict on the calls edges out of caller
// at rev, each against its callee's signature at rev.
func CallerConflicts(g Reader, caller graph.ID, rev graph.Rev) []Conflict {
	var out []Conflict
	for _, e := range g.Out(caller, rev, graph.Calls) {
		for _, c := range ArityConflicts(g, e.Dst, rev) {
			if c.Edge.Src == caller {
				out = append(out, c)
			}
		}
	}
	return out
}

// arity is the callee's admitted argument range: max is -1 for a variadic
// signature. known is false when the node carries no arity.
func arity(n graph.Node) (min, max int, known bool) {
	min, err := strconv.Atoi(n.Attrs["arity_min"])
	if err != nil {
		return 0, 0, false
	}
	if n.Attrs["arity_max"] == "*" {
		return min, -1, true
	}
	max, err = strconv.Atoi(n.Attrs["arity_max"])
	if err != nil {
		return 0, 0, false
	}
	return min, max, true
}
