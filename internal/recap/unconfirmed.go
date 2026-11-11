package recap

import (
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// unconfirmedRow is the unconfirmed_write row's sentence (verify.TurnSummary's
// guard): what was measured, then the call that did not end ok and how the
// record holds its end, after files where the turn edited. Never "verified",
// and never "no edits", which a call that may have written makes false. A
// turn that ran no gate names the call and the gates not run after it. Its
// strings are held to FUN-15's list as value.go's are (TestFUN15b_).
func (t gateSet) unconfirmedRow(v verify.StopView, k int, files string) string {
	if v.Summary.GreenSeq == 0 {
		return unconfirmed(v.Summary.Unconfirmed, v.Summary.Ending) + "; " + t.notRunAfter()
	}
	greens, _ := t.passed(k)
	s := greens + " (call " + t.greenSeq() + ")" + t.fromOutput() + "; " + unconfirmed(v.Summary.Unconfirmed, v.Summary.Ending)
	if v.Summary.Ending != verify.EndNone {
		s += t.placed(v.Summary.Unconfirmed, v.Summary.GreenSeq)
	}
	if v.Turn.Writes > 0 {
		s = files + "; " + s
	}
	return s
}

// unconfirmed is the clause that names seq, a call that did not end ok, in
// the closed words for how the record holds its end, end. A failed file tool
// is named an edit that failed, which says it did not apply; a failed shell
// or MCP call, and an interrupted call, may have written; a call with no
// ending is placed nowhere, since it may have written before the green or
// after it.
func unconfirmed(seq int64, end verify.WriteEnd) string {
	call := "call " + strconv.FormatInt(seq, 10)
	switch end {
	case verify.EndFailedEdit:
		return call + ", an edit, failed"
	case verify.EndFailedCall:
		return call + " failed and may have written files"
	case verify.EndInterrupted:
		return call + " was interrupted and may have written files"
	}
	return call + " has no recorded ending"
}

// placed puts call against the green runs the line names: "after that"
// past the latest of them, which "(call <greenSeq>)" names; "before that"
// before green, the earliest, so before every one; and between the two,
// after the gate whose last green green is, by name and call, since "after
// that" would place it after a run it came before (two gates). green is the
// run the guard read the call against (Summary.GreenSeq).
func (t gateSet) placed(call, green int64) string {
	latest := t.latestGreen()
	switch {
	case call > latest:
		return " after that"
	case call < green:
		return " before that"
	}
	for i := range t.gates {
		if g := &t.gates[i]; g.State == verify.Green && g.LastGreen != nil && g.LastGreen.Seq == green {
			return " after " + t.name(g) + " passed (call " + strconv.FormatInt(green, 10) + ")"
		}
	}
	return " after call " + strconv.FormatInt(green, 10)
}

// wroteAfter is the clause a row adds where it places run, a gate's run,
// after the last edit while a call that may have written stands after it
// (verify.TurnCounts.WroteAfter), "" for none. "After the last edit" counts
// edits alone, so without the clause the line reads as if the run saw the
// turn's last change, which the record did not measure. after places the
// call against run where the sentence has just named it; "" places it by
// the green runs the row names (passedBefore). A call with no ending is
// placed nowhere, as unconfirmed says. The cost: a longer line, on which
// fitted drops gate names sooner.
func (t gateSet) wroteAfter(v verify.StopView, run int64, after string) string {
	seq, end := v.Turn.WroteAfter(run)
	switch {
	case seq == 0:
		return ""
	case end == verify.EndNone:
		after = ""
	case after == "":
		after = t.passedBefore(seq)
	}
	return "; " + unconfirmed(seq, end) + after
}

// heldPast is wroteAfter for g's last run where the row placed it after the
// last edit (when): a run placed before it already says it has not run
// since, and one placed in a turn with no edit says nothing of an edit.
func (t gateSet) heldPast(v verify.StopView, g *verify.GateState, where, after string) string {
	if where != afterLastEdit || g.LastRun == nil {
		return ""
	}
	return t.wroteAfter(v, g.LastRun.Seq, after)
}

// passedBefore places call after the latest green run before it, by gate
// name and call, for a row that names its greens without their calls
// (unverified's "passed after the last edit").
func (t gateSet) passedBefore(call int64) string {
	var at *verify.GateState
	for i := range t.gates {
		g := &t.gates[i]
		if g.State == verify.Green && g.LastGreen != nil && g.LastGreen.Seq < call && (at == nil || g.LastGreen.Seq > at.LastGreen.Seq) {
			at = g
		}
	}
	if at == nil {
		return ""
	}
	return " after " + t.name(at) + " passed (call " + strconv.FormatInt(at.LastGreen.Seq, 10) + ")"
}

// earliestGreen is the earliest last green of the green turn gates, the run
// TurnSummary reads a held call against; 0 for none.
func (t gateSet) earliestGreen() int64 {
	var at int64
	for _, g := range t.gates {
		if g.State == verify.Green && g.LastGreen != nil && (at == 0 || g.LastGreen.Seq < at) {
			at = g.LastGreen.Seq
		}
	}
	return at
}

// notRunAfter is the turn's gates, each by name, not run after the call the
// row names: the turn ran none. With no gate, no test or build ran either.
func (t gateSet) notRunAfter() string {
	var names []string
	for i := range t.gates {
		if n := t.name(&t.gates[i]); !contains(names, n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "no test or build run after it"
	}
	return joinNames(names) + " not run after it"
}
