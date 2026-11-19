package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestRH8j_ClaimUnbackedIsTheDoneFlagsPredicate (item 8): the report's
// flagged-claims line reads ClaimUnbacked, and the done flag reads unbacked;
// on every verdict, a state of verify.Order, no_gate_known and a word outside
// the vocabulary, strict and loose, the two agree on one claim, so the line
// and the flag cannot disagree. Break: flag a loose claim, a green one or a
// no_gate_known one, or none.
func TestRH8j_ClaimUnbackedIsTheDoneFlagsPredicate(t *testing.T) {
	verdicts := []verify.Verdict{verify.NoGateKnown, "not_a_state"}
	for _, s := range verify.Order {
		verdicts = append(verdicts, verify.Verdict(s))
	}
	flagged := 0
	for _, v := range verdicts {
		for _, strict := range []bool{false, true} {
			c := verify.ClaimVerdict{Class: verify.ClaimTestsPass, Strict: strict, Verdict: v}
			want := unbacked([]verify.ClaimVerdict{c})
			if got := ClaimUnbacked(c); got != want {
				t.Errorf("verdict %q strict %v: ClaimUnbacked %v, the done flag's unbacked %v", v, strict, got, want)
			}
			if want {
				flagged++
			}
		}
	}
	if flagged != len(verify.Order)-1 {
		t.Errorf("premise: the done flag reads %d of the claims as unbacked, want the %d strict ones on a state other than green", flagged, len(verify.Order)-1)
	}
}
