package recap

import "github.com/altrace-dev-role/rashomon/internal/verify"

// ClaimUnbacked reports whether the done flag reads c as unbacked: a strict
// claim whose verdict is a gate state other than green. It is unbacked's
// predicate for one claim, and the report's strict-claims line reads it. It
// is one term of four in the done flag's decision: the flag also reads the
// row, the row's sentence and the unmet expected kinds, so the line and the
// flag can disagree. TestRH8j pins this term equal to the flag's on every
// verdict.
func ClaimUnbacked(c verify.ClaimVerdict) bool {
	return c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green)
}
