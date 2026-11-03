package recap

import (
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// Mistakes against intent (card REPORT-REQS1, INTENT-MISTAKES): the session
// report's count line, "k caught at the act, p reached the person, r found
// later", read from what the rows record and never a score. A mistake the
// record holds is a finding line, delivered or not; each is classified once,
// by the first rule that applies, so the lists partition the findings and
// every count is a list of findings with their seqs. Whether the agent was
// told is read from delivery (ImpactOf's Delivered), never from a finding's
// state: a line withheld by a cap or a closed channel and then resolved was
// fixed with no line, not caught.

// Reach is why a finding reached the person.
type Reach string

const (
	// ReachedAtStop: its line was delivered at a Stop, which speaks to the
	// person alone (one systemMessage, R2b), so the agent was not told at the
	// act, even when a later turn resolved it.
	ReachedAtStop Reach = "stop"
	// ReachedOpen: the agent was told and no green run of the line's gate
	// came after it, and the last printed clause counted it (its counts are
	// the impact's now).
	ReachedOpen Reach = "open"
	// ReachedFlag: the line was delivered as a flag, to the person alone (no
	// model text went out), and no green run of its gate came after it.
	ReachedFlag Reach = "flag"
)

// ReachedRef is a finding that reached the person, and why.
type ReachedRef struct {
	FindingRef
	Why Reach
}

// Mistakes is the session's findings partitioned:
//   - Caught: told to the agent at a call or a prompt, then resolved by a
//     green run of the gate the line names.
//   - Reached: got to the person.
//   - Missed: still open at the last Stop, and either never told or told and
//     never shown to the person. This is the honest miss count. A nudge on a
//     client below its event's floor is never delivered (decide suppresses
//     it as unverified_channel), so it lands here or in Fixed.
//   - Unsettled: names no gate, so no run settles it (ADHERE1's complied or
//     ignored will).
//   - Fixed: never told, then resolved: the agent fixed it on its own. Listed,
//     never counted as caught.
//
// Nothing records a mistake found after the session (LOOP-TRACE1), so that
// list is not here: a reader reports it unknown, never 0.
type Mistakes struct {
	Caught    []FindingRef
	Reached   []ReachedRef
	Missed    []FindingRef
	Unsettled []FindingRef
	Fixed     []FindingRef
}

// MistakesOf is the mistakes of the session whose Stop view is v, whose steer
// lines are lines and whose impact is im (ImpactOf over the same two). shown
// is the fingerprint of the counts the session's last printed clause carried
// (ImpactShown), "" with none.
func MistakesOf(v verify.StopView, lines [][]byte, im Impact, shown string) Mistakes {
	return classify(foundOf(v, lines, im), shown != "" && shown == im.Fingerprint())
}

// found is one finding line the record holds and who was told at the act:
// told, the agent (its id is in Delivered, so a model text went out) or the
// person at a Stop; flagged, the person alone (a flag: delivered with a user
// text and no model text). A held-out line is delivered with neither text
// (decide's holdOut), so it told no one.
type found struct {
	FindingRef
	told, flagged bool
}

// rank orders a finding's occurrences: told over flagged over withheld.
func (f found) rank() int {
	switch {
	case f.told:
		return 2
	case f.flagged:
		return 1
	}
	return 0
}

// foundOf is every finding line of lines, delivered or withheld, once per id
// in record order, the occurrence that told the most kept (rank). A superseded
// line is passed over: the line that superseded it is the finding. A line
// with no id is never folded into another, as ImpactOf counts it.
func foundOf(v verify.StopView, lines [][]byte, im Impact) []found {
	told := map[string]bool{}
	for _, n := range im.Delivered {
		told[n.ID] = true
	}
	var out []found
	at := map[string]int{}
	var sp [lkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, lineKeys(), sp[:]) != nil {
			continue
		}
		pat, state := text(b, sp[lkPattern]), text(b, sp[lkState])
		if !findingPatterns()[pat] || state == string(steer.LineStateSuperseded) {
			continue
		}
		id := text(b, sp[lkID])
		f := found{FindingRef: findingOf(v, b, sp[:], id, pat)}
		delivered := state == string(steer.LineStateDelivered)
		f.told = delivered && (told[id] || atStop(f.Event))
		f.flagged = delivered && !f.told && text(b, sp[lkUserText]) != ""
		i, seen := at[id]
		switch {
		case !seen || id == "":
			at[id] = len(out)
			out = append(out, f)
		case f.rank() > out[i].rank():
			out[i] = f
		}
	}
	return out
}

// classify applies the rules in order. carried says the last printed clause
// counted every finding told to the agent.
func classify(fs []found, carried bool) Mistakes {
	var m Mistakes
	for _, f := range fs {
		switch {
		case f.told && atStop(f.Event):
			m.Reached = append(m.Reached, ReachedRef{FindingRef: f.FindingRef, Why: ReachedAtStop})
		case f.State == FindingUnchecked:
			m.Unsettled = append(m.Unsettled, f.FindingRef)
		case f.told && f.State == FindingResolved:
			m.Caught = append(m.Caught, f.FindingRef)
		case f.State == FindingResolved:
			m.Fixed = append(m.Fixed, f.FindingRef)
		case f.told && carried:
			m.Reached = append(m.Reached, ReachedRef{FindingRef: f.FindingRef, Why: ReachedOpen})
		case f.flagged:
			m.Reached = append(m.Reached, ReachedRef{FindingRef: f.FindingRef, Why: ReachedFlag})
		default:
			m.Missed = append(m.Missed, f.FindingRef)
		}
	}
	return m
}

// atStop is whether event is a turn end's, whose line speaks to the person.
func atStop(event string) bool {
	return event == install.EventStop || event == install.EventStopFailure
}
