package main

import (
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// owedNote is what the Stop view v leaves the session's next prompt (card
// GATE-INFER1, W5). The zero Owed is none.
func owedNote(v verify.StopView, inf *verify.InferredGate) recap.Owed {
	return recap.Owed{}
}

// owedFinding is the next prompt's note for o.
func owedFinding(o recap.Owed, afterSeq int64, lines [][]byte) (steer.Finding, bool) {
	return steer.Finding{}, false
}
