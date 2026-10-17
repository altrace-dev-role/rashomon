package budget

import "testing"

// TestRubricNAMING12_TheZeroPhaseIsNoPhase holds Phase's zero value neutral:
// an unset Phase is not Verify, has no stand-down word -- the empty string,
// which no writer may record -- and is not listed among the phases.
func TestRubricNAMING12_TheZeroPhaseIsNoPhase(t *testing.T) {
	var zero Phase
	if zero == Verify {
		t.Error("the zero Phase is Verify; an unset phase would stand down as verify_budget")
	}
	if got := zero.Reason(); got != "" {
		t.Errorf("the zero Phase has reason %q; it must have none", got)
	}
	for _, p := range Phases() {
		if p == zero {
			t.Error("Phases lists the zero Phase")
		}
	}
}
