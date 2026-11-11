package spec

// The predicates a decision rule may name in Predicate. Data only: Parse
// checks Predicate's shape, not membership; the check against this list
// lands with the compiler that writes the field.
const (
	PredicatePathsUntouched = "paths_untouched"
	PredicateSymbolPresent  = "symbol_present"
)

// RulePredicates is the closed list of decision predicates.
var RulePredicates = []string{PredicatePathsUntouched, PredicateSymbolPresent}

// UnenforcedRuleKinds is a known limit: kinds Parse accepts that no
// evaluator enforces yet. A rule of one of them changes no verdict, ledger
// row or finding; the prompt reader names it only when its Paths cover the
// prompt's file, as for any path rule. Each card that enforces a kind
// deletes it here in the same commit, and the milestone's close reads this
// list, so an accepted-but-inert kind is never silent.
var UnenforcedRuleKinds = []RuleKind{Boundary, Decision, Reuse}

// ruleKindNames is RuleKinds as the strings Parse checks a rule's kind
// against. It converts on every call, not into a package variable, so
// RuleKinds stays the one list and nothing derived is built at load (RES-2).
func ruleKindNames() []string {
	out := make([]string, len(RuleKinds))
	for i, k := range RuleKinds {
		out[i] = string(k)
	}
	return out
}

// ruleRefs reads the fields the knowledge graph's kinds carry: from, to and
// applies_to as lists of graph ids, predicate and source as plain text. Parse
// checks their shape only; which kind needs which field is the evaluator's
// call, as for the table's five (Rule).
func (r *reader) ruleRefs(f fields, x *Rule) {
	x.From = field(r, f, "from", false, r.words)
	x.To = field(r, f, "to", false, r.words)
	x.Predicate = field(r, f, "predicate", false, r.plain)
	x.AppliesTo = field(r, f, "applies_to", false, r.words)
	x.Source = field(r, f, "source", false, r.plain)
}
