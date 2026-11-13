package engine

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/spec"
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

// Key is the conflict's identity across recomputations. An arity
// conflict's is its edge's key byte for byte (the session keys its arity
// conflicts by it); any other kind's is its kind, rule, unit and subject,
// and its edge's key when it has an edge. Evidence is not identity.
func (c Conflict) Key() string {
	if c.Kind == ConflictArity {
		return c.Edge.Key()
	}
	k := strings.Join([]string{string(c.Kind), c.RuleID, string(c.Unit), string(c.Subject)}, "\x00")
	if c.Edge.Src != "" {
		k += "\x00" + c.Edge.Key()
	}
	return k
}

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

// Conflicts is every conflict the diff raises at d.Rev: each rule's, for
// the kinds boundary, decision and protect_path (a rule of any other kind
// raises none), then signature_mismatch and duplicate_abstraction, which no
// rule raises. The order is the kind's place in ConflictKinds, then Unit,
// RuleID, Subject and the edge's key, whatever the order of the rules or
// of the diff's records; of conflicts with one key, the first is kept.
func Conflicts(g Reader, d Diff, rules []Rule) []Conflict {
	var out []Conflict
	for _, r := range rules {
		switch spec.RuleKind(r.Kind) {
		case spec.Boundary:
			out = append(out, boundaryConflicts(g, d, r)...)
		case spec.Decision:
			out = append(out, decisionConflicts(g, d, r)...)
		case spec.ProtectPath:
			out = append(out, protectedConflicts(g, d, r)...)
		}
	}
	out = append(out, signatureConflicts(g, d)...)
	out = append(out, duplicateConflicts(g, d)...)
	slices.SortStableFunc(out, func(a, b Conflict) int {
		return cmp.Or(cmp.Compare(slices.Index(ConflictKinds, a.Kind), slices.Index(ConflictKinds, b.Kind)),
			cmp.Compare(a.Unit, b.Unit), cmp.Compare(a.RuleID, b.RuleID), cmp.Compare(a.Subject, b.Subject),
			cmp.Compare(a.Edge.Key(), b.Edge.Key()))
	})
	return slices.CompactFunc(out, func(a, b Conflict) bool { return a.Key() == b.Key() })
}

// ruleConflict is a conflict of kind k that rule r raises about id, in
// id's unit, with id its evidence.
func ruleConflict(g Reader, d Diff, r Rule, k ConflictKind, id graph.ID) Conflict {
	return Conflict{Kind: k, Rev: d.Rev, Unit: unitAt(g, d, id), RuleID: r.ID, Subject: id, Evidence: []graph.ID{id}}
}

// unitIDOf is the unit holding id at rev: a unit's own id, a symbol's from
// its id, a file's through the containing edge; empty for the rest.
func unitIDOf(g Reader, rev graph.Rev, id graph.ID) graph.ID {
	if strings.HasPrefix(string(id), "unit:") {
		return id
	}
	_, u := unitOf(g, rev, graph.Node{ID: id})
	return u
}

// unitAt is the unit holding id at d.Rev, or at d.Base for an id gone at
// d.Rev, so a file the diff deletes keeps its unit.
func unitAt(g Reader, d Diff, id graph.ID) graph.ID {
	if u := unitIDOf(g, d.Rev, id); u != "" {
		return u
	}
	return unitIDOf(g, d.Base, id)
}

// removedIDs is the ids of the nodes the diff removes.
func removedIDs(d Diff) map[graph.ID]bool {
	out := map[graph.ID]bool{}
	for _, r := range d.Removed {
		if r.Node != nil {
			out[r.Node.ID] = true
		}
	}
	return out
}
