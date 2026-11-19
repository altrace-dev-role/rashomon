package recap

import "github.com/altrace-dev-role/rashomon/internal/verify"

// ClaimUnbacked reports whether the done flag reads c as unbacked: a strict
// claim whose verdict is a gate state other than green. It is unbacked's
// predicate for one claim, which the report's flagged-claims line reads, so
// the line and the flag cannot disagree; TestRH8j pins the two equal on every
// verdict.
func ClaimUnbacked(c verify.ClaimVerdict) bool {
	return c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green)
}
