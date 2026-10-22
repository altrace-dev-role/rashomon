package spend

import "testing"

// TestRubricNAMING6_WhollyUnpricedIsTrueOnlyWithNoPricedPart holds the name to
// what it answers: a cost is wholly unpriced when responses stand behind it
// and none of them was priced. A partly priced cost has a priced part to show,
// and a cost with no responses behind it is a known zero.
func TestRubricNAMING6_WhollyUnpricedIsTrueOnlyWithNoPricedPart(t *testing.T) {
	for _, c := range []struct {
		cost Cost
		want bool
	}{
		{Cost{}, false},
		{Cost{Priced: 2, Nano: 5}, false},
		{Cost{Priced: 1, Unpriced: 1, Nano: 5, UnpricedTokens: 9}, false},
		{Cost{Unpriced: 3, UnpricedTokens: 9}, true},
	} {
		if got := c.cost.WhollyUnpriced(); got != c.want {
			t.Errorf("%+v.WhollyUnpriced() = %v, want %v", c.cost, got, c.want)
		}
	}
}
