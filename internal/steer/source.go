package steer

import (
	"fmt"
	"slices"
	"strings"
)

// Source is where a Finding's claim came from (AD19, Part B6): the store
// class, the row's global id in it (g:<rid>#<local>, graph.GlobalID's
// form), and the row's tier as the store spells it. The zero Source is a
// finding of the record-era patterns, which cite record seqs (Evidence)
// instead. M5's label writer reads Kind and Source to pick the compliance
// predicate; the steer line carries them from W-M5.1.
type Source struct {
	Kind string
	ID   string
	Tier string
}

// The source kinds: the three stores by epistemic class (AD1), the record,
// and the proposer layer (AD16).
const (
	SourceGraph     = "graph"
	SourceKnowledge = "knowledge"
	SourceSpec      = "spec"
	SourceRecord    = "record"
	SourceProposer  = "proposer"
)

// SourceKinds is the closed list, a fresh slice each call.
func SourceKinds() []string {
	return []string{SourceGraph, SourceKnowledge, SourceSpec, SourceRecord, SourceProposer}
}

// check is Source's part of Finding.Check: empty, or a listed kind with a
// global id.
func (s Source) check() error {
	switch {
	case s == Source{}:
		return nil
	case !slices.Contains(SourceKinds(), s.Kind):
		return fmt.Errorf("%w: source kind %q is outside its vocabulary", ErrFinding, s.Kind)
	case !isGlobal(s.ID, ""):
		return fmt.Errorf("%w: source id is not a global id", ErrFinding)
	}
	return nil
}

// isGlobal reports a global id, g:<rid>#<local> with both parts present,
// whose local part starts with prefix.
func isGlobal(id, prefix string) bool {
	rest, ok := strings.CutPrefix(id, "g:")
	if !ok {
		return false
	}
	rid, local, ok := strings.Cut(rest, "#")
	return ok && rid != "" && local != "" && strings.HasPrefix(local, prefix)
}

// The graph line kinds (Part B6): what a line delivered from the graph
// claims, each judged at label time by its compliance predicates
// (Compliance). They become frozen kinds of the graph patterns when M2
// delivers one finding per line; M1's brief is one task-preflight finding
// over up to five lines and carries no kind.
const (
	KindUnit       Kind = "unit"
	KindReuse      Kind = "reuse"
	KindConsumer   Kind = "consumer"
	KindDecision   Kind = "decision"
	KindLesson     Kind = "lesson"
	KindConflict   Kind = "conflict"
	KindObligation Kind = "obligation"
	KindBoundary   Kind = "boundary"
	KindDrift      Kind = "drift"
	KindFlow       Kind = "flow"
)

// GraphKinds is the closed list, a fresh slice each call.
func GraphKinds() []Kind {
	return []Kind{KindUnit, KindReuse, KindConsumer, KindDecision, KindLesson, KindConflict, KindObligation, KindBoundary, KindDrift, KindFlow}
}

// The compliance predicates: how a delivered line of each kind is judged
// followed (D, never A) from the record and the tree, by name, so a label
// says which predicate held. A lesson's predicate is the lesson row's own
// (knowledge.Row.Predicates); a boundary line has none to hold.
const (
	PredOpenedUnit        = "opened_unit"
	PredCalledSymbol      = "called_symbol"
	PredEditedOrVerified  = "edited_or_verified"
	PredPathsUntouched    = "paths_untouched"
	PredSymbolPresent     = "symbol_present"
	PredLessonOwn         = "lesson_own"
	PredResolvedByStop    = "resolved_by_stop"
	PredMet               = "met"
	PredNone              = "none"
	PredReturnedToScope   = "returned_to_scope"
	PredConsumersVerified = "consumers_verified"
)

// Compliance is the predicates a line of kind k is judged by, in the order
// they are tried, and whether k is a graph kind.
func Compliance(k Kind) ([]string, bool) {
	switch k {
	case KindUnit:
		return []string{PredOpenedUnit}, true
	case KindReuse:
		return []string{PredCalledSymbol}, true
	case KindConsumer:
		return []string{PredEditedOrVerified}, true
	case KindDecision:
		return []string{PredPathsUntouched, PredSymbolPresent}, true
	case KindLesson:
		return []string{PredLessonOwn}, true
	case KindConflict:
		return []string{PredResolvedByStop}, true
	case KindObligation:
		return []string{PredMet}, true
	case KindBoundary:
		return []string{PredNone}, true
	case KindDrift:
		return []string{PredReturnedToScope}, true
	case KindFlow:
		return []string{PredConsumersVerified}, true
	}
	return nil, false
}
