package slot

import (
	"strings"
	"testing"
)

// TestTB1_ListsIsWhatALineNames (card TOKEN-BUDGET1): a list lists when it
// names an item, listed, left out by a check, digested or counted in its
// record form, and not when it holds only counts of items given earlier,
// withheld or learned, which say nothing new to the model. Break: count a
// tail as a listing, or miss the left-out and record forms, whose lines
// would then differ from the printed form's.
func TestTB1_ListsIsWhatALineNames(t *testing.T) {
	trusted := NewEnum(EnumTrust, "trusted")
	for _, c := range []struct {
		name string
		l    Lister
		want bool
	}{
		{"no path", NewPathSet(nil, 0, 0), false},
		{"paths given earlier", NewPathSet(nil, 0, 2), false},
		{"a path", NewPathSet([]Path{"a.go"}, 0, 0), true},
		{"a path left out", NewPathSet(nil, 1, 0), true},
		{"a path's digest", NewDigestPathSet([]string{strings.Repeat("0", 64)}, 0, 0), true},
		{"rules given earlier and withheld", NewRuleRefs(nil, 0, 1, 2), false},
		{"a rule", NewRuleRefs([]RuleRef{{ID: "r1", Trust: trusted}}, 0, 0, 0), true},
		{"a rule left out", NewRuleRefs(nil, 1, 0, 0), true},
		{"gates learned", NewGateRefs(nil, 0, 3), false},
		{"no gate index", NewGateRefs(nil, 0, -1), false},
		{"a gate", NewGateRefs([]GateRef{{Words: "go test", Trust: trusted}}, 0, -1), true},
		{"no value", NewEnumList(EnumEvidenceKinds, nil), false},
		{"a value", NewEnumList(EnumEvidenceKinds, []string{"cross_check"}), true},
		{"no symbol", NewSymbolFacts(nil, 0), false},
		{"a symbol", NewSymbolFacts([]SymbolFact{{Name: "CountRows", Dir: "a", Callers: 1, Packages: 1}}, 0), true},
		{"symbols counted", NewSymbolFacts([]SymbolFact{{Name: "CountRows", Dir: "a"}}, 1).Counted(), true},
		{"no count", Count(0), false},
		{"a count", Count(3), true},
	} {
		if got := c.l.Lists(); got != c.want {
			t.Errorf("%s: Lists() = %v, want %v", c.name, got, c.want)
		}
	}
}
