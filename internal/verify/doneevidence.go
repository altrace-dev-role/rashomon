package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
)

// DoneEvidence is a digest of the evidence the done flag (recap.DoneFlag)
// decides on at this Stop: the row and the last write seq of the window the
// verdict judged (after the swap to Done, as DoneFlag reads it), each turn
// gate (TurnGate under Advisory: a required gate on the turn, or, where no
// gate is required, a learned one) with its last run's seq and its state,
// each strict claim with its verdict, and, under a strict claim of done, each
// unmet expected kind with its reason. Two Stops with one digest flagged the
// same evidence, so the note to the agent and the flag on the person's line
// are each given once per digest. The point's seq, the session's owed row, a
// loose claim and a repeated claim are no input. The gate set is as wide as
// both sets the flag reads: the row's and its sentence's (TurnGate), and the
// claim verdicts' (mapGates, the required gates on the turn, which it keeps
// even where one owes nothing). A wider set can only say a note again, never
// swallow one.
//
// It is 16 hex digits of a sha256, so no gate name is stored. Cost: one sort
// and one sha256 over a short string; the Stop builds it only where the flag
// fires.
func DoneEvidence(v StopView) string {
	if v.Done != nil {
		v.Summary, v.Turn = v.Done.Summary, v.Done.Turn
	}
	parts := []string{lenField("v1"), lenField("r") + lenField(string(v.Summary.Row)),
		lenField("w") + lenField(strconv.FormatInt(v.Turn.LastWriteSeq, 10))}
	advisory := Advisory(v.Snap.Gates)
	for i := range v.Snap.Gates {
		g := &v.Snap.Gates[i]
		if !TurnGate(*g, advisory) {
			continue
		}
		var seq int64
		if g.LastRun != nil {
			seq = g.LastRun.Seq
		}
		parts = append(parts, lenField("g")+lenField(g.Gate)+lenField(strconv.FormatInt(seq, 10))+lenField(string(g.State)))
	}
	done := false
	for _, c := range v.Scan.Claims {
		if !c.Strict {
			continue
		}
		done = done || c.Class == ClaimDone
		parts = append(parts, lenField("c")+lenField(string(c.Class))+lenField(string(c.Verdict)))
	}
	if done {
		for _, e := range UnmetExpected(v.Expected, v.Obligations) {
			parts = append(parts, lenField("u")+lenField(e.Kind)+lenField(e.Reason))
		}
	}
	slices.Sort(parts)
	parts = slices.Compact(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:8])
}

// lenField is s with its length in front, so no two lists of parts join to
// one string.
func lenField(s string) string { return strconv.Itoa(len(s)) + ":" + s }
