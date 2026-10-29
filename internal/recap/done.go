package recap

import "github.com/altrace-dev-role/rashomon/internal/verify"

// doneLead opens the done flag's sentence. It names the claim class "done",
// which FUN-15 keeps off the value line's templates, so it lives here and not
// in value.go.
const doneLead = "done was claimed; the record shows "

// noRunSentence is the done flag's value sentence where the row names no gate
// run: RowNone and no_gate.
const noRunSentence = "no test or build run is recorded for this turn"

// DoneFlag renders the done flag (PF-PG9; card 2.4 R7): a strict claim in the
// final message whose verdict is a state other than green, on a turn whose
// row is not verified, takes the value line's place with the row's sentence.
// no_gate_known is no state and does not fire it: a claim no gate speaks to
// contradicts nothing (D79), as steer's claim kinds read it.
func DoneFlag(v verify.StopView, sessionID string, fromPlugin bool) (string, bool) {
	if v.Summary.Row == verify.RowVerified || !unbacked(v.Scan.Claims) {
		return "", false
	}
	s := noRunSentence
	if v.Summary.Row != verify.RowNone && v.Summary.Row != verify.RowNoGate {
		if f := fitted(v, prefix+doneLead); f != "" {
			s = f
		}
	}
	return prefix + doneLead + s + ".\n" + pointer(sessionID, fromPlugin), true
}

// unbacked reports whether a strict claim's verdict is a state other than
// green.
func unbacked(claims []verify.ClaimVerdict) bool {
	for _, c := range claims {
		if c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green) {
			return true
		}
	}
	return false
}
