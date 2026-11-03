package slot

// Lister is a preflight list that can name nothing (TOKEN-BUDGET1): a sparse
// template leaves out of a message's text a line whose every Lister slot
// lists nothing (render). A count of items given earlier, withheld from an
// untrusted spec or learned from earlier sessions names nothing new, so it
// lists nothing; an item left out by a check was named, so it lists, and the
// record form, which counts or digests what the printed form names, lists
// exactly when the printed form does.
type Lister interface {
	Lists() bool
}

// Lists reports a named path, listed, left out or digested.
func (s PathSet) Lists() bool { return len(s.paths)+len(s.digests)+s.more > 0 }

// Lists reports a rule that applies, listed or left out.
func (r RuleRefs) Lists() bool { return len(r.items)+r.more > 0 }

// Lists reports a gate, listed or left out.
func (g GateRefs) Lists() bool { return len(g.items)+g.more > 0 }

// Lists reports a value.
func (l EnumList) Lists() bool { return len(l.values) > 0 }

// Lists reports a proven symbol, listed, left out or counted.
func (s SymbolFacts) Lists() bool { return len(s.items)+s.more > 0 }

// Lists reports a count above zero.
func (c Count) Lists() bool { return c > 0 }
