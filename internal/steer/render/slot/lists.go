package slot

// Lister is a preflight list that can name nothing (TOKEN-BUDGET1).
type Lister interface {
	Lists() bool
}

// Lists reports a named path.
func (s PathSet) Lists() bool { return false }

// Lists reports a rule that applies.
func (r RuleRefs) Lists() bool { return false }

// Lists reports a gate.
func (g GateRefs) Lists() bool { return false }

// Lists reports a value.
func (l EnumList) Lists() bool { return false }

// Lists reports a proven symbol.
func (s SymbolFacts) Lists() bool { return false }

// Lists reports a count above zero.
func (c Count) Lists() bool { return false }
