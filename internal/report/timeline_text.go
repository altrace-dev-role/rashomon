package report

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// timelineRows caps the text listing. Past it the rest is counted, never
// silently cut; JSON always carries every call.
const timelineRows = 500

// writeTimeline renders the timeline: the counts first, so a reader sees what
// failed and what never ran before scrolling, then one row per call.
func writeTimeline(b *bytes.Buffer, t Timeline) {
	n := t.Counts
	fmt.Fprintf(b, "  timeline: %d call%s (%s), in the order they were recorded\n",
		n.Calls, plural(n.Calls), timelineWho(n))
	if n.Calls == 0 {
		return
	}
	fmt.Fprintf(b, "    ok           %d\n", n.OK)
	// "Recorded after", not "later": the comparison is between execution
	// records' positions, which is when each result was written down.
	notChecked := ""
	if n.NotChecked > 0 {
		notChecked = fmt.Sprintf(", %d not checked", n.NotChecked)
	}
	fmt.Fprintf(b, "    failed       %d  (%d same command ok, recorded after; %d same program ok, recorded after; %d no later success of the same command or program recorded%s)\n",
		n.Failed, n.SameCommand, n.SameProgram, n.NoLater, notChecked)
	// What the follow-up does NOT look for, said once: a failure fixed some
	// other way reads as having none, and a reader must not take that row as
	// a failure nobody dealt with.
	if n.Failed > 0 {
		fmt.Fprintln(b, "                 (only a later run of the same command, or of the same program for single-purpose programs, is looked for; "+
			"for wrappers and multi-command programs such as git, go, make, npm, python and sudo only the same command is; "+
			"a fix made with a different command, or a corrected Edit, is not detected)")
	}
	if n.Interrupted > 0 {
		fmt.Fprintf(b, "    interrupted  %d\n", n.Interrupted)
	}
	fmt.Fprintf(b, "    never ran    %d  (denied before running)\n", n.NeverRan)
	// Both things the group holds, named: a row that HAS an execution record
	// with no outcome would otherwise sit under a legend saying it has none.
	fmt.Fprintf(b, "    unknown      %d  (no execution record: denied, failed or unrecorded, and the record cannot say which; or outcome unobserved: it ran and how it ended was not recorded)\n", n.Unknown)
	if n.AgentUnknown > 0 {
		fmt.Fprintf(b, "    %d call%s with no declaration recorded, listed last in the order their results were recorded: agent, program, and the declaration's position and time unknown\n",
			n.AgentUnknown, plural(n.AgentUnknown))
	}
	if n.Subagents > 0 {
		fmt.Fprintln(b, "    calls from agents running at once interleave by when each was recorded, not when it started")
	}

	// Seq is rashomon's own store position, which no transcript shows; the
	// tool_use_id is what joins a row to a transcript or to the report's
	// "executed differently from declared" list. The row layout stays as it
	// is, so the reader is told where the id is.
	fmt.Fprintln(b, "    rows omit tool_use_id; --json carries it for every call")
	// The time column is UTC and carries no date, so the date is printed
	// above the first row and again wherever it changes.
	fmt.Fprintf(b, "    %5s  %-8s  %-22s %-22s %s\n", "seq", "UTC", "agent", "call", "result")
	day := ""
	for i, c := range t.Calls {
		if i == timelineRows {
			rest := len(t.Calls) - timelineRows
			fmt.Fprintf(b, "    %d more call%s, see --json\n", rest, plural(rest))
			break
		}
		if c.RecordedAtMS != nil {
			if d := time.UnixMilli(*c.RecordedAtMS).UTC().Format("2006-01-02"); d != day {
				day = d
				fmt.Fprintf(b, "    %s (UTC)\n", day)
			}
		}
		writeTimelineCall(b, c)
	}
}

// timelineWho says who made the calls, from the counts: the main agent is
// named with its own number, so one that made none reads as 0 rather than as
// a participant.
func timelineWho(n TimelineCounts) string {
	who := fmt.Sprintf("%d main agent", n.MainAgent)
	if n.Subagents > 0 {
		sub := n.Calls - n.MainAgent - n.AgentUnknown
		who += fmt.Sprintf(", %d from %d subagent%s", sub, n.Subagents, plural(n.Subagents))
	}
	if n.AgentUnknown > 0 {
		who += fmt.Sprintf(", %d agent unknown", n.AgentUnknown)
	}
	return who
}

func writeTimelineCall(b *bytes.Buffer, c TimelineCall) {
	seq, at := "-", "--:--:--"
	if c.Seq != nil {
		seq = fmt.Sprint(*c.Seq)
	}
	if c.RecordedAtMS != nil {
		at = time.UnixMilli(*c.RecordedAtMS).UTC().Format("15:04:05")
	}
	// tool_name and the program are the payload's, as agent_type is: made
	// printable, or an escape sequence in one reaches the reader's terminal
	// on its row.
	call := printable(c.ToolName)
	if c.Program != "" {
		call += " " + printable(c.Program)
	}
	agent := agentLabel(c.Agent)
	if c.AgentUnknown {
		agent = LinkUnknown
	}
	fmt.Fprintf(b, "    %5s  %s  %-22s %-22s %s%s\n",
		seq, at, agent, call, timelineResult(c), laterLabel(c))
}

// agentLabel names the agent a call ran in: "main", or the subagent's type
// with the first four characters of its id, which keeps two subagents of one
// type apart without printing ids a reader cannot use. An "agent-" prefix is
// dropped first, or every label would read "agen".
//
// Both come from the hook payload as the client sent them, so both are made
// printable first: a control byte in agent_type would otherwise reach the
// reader's terminal on every row. The id is cut by rune, not byte, so the
// cut never splits a character.
func agentLabel(a *TimelineAgent) string {
	if a == nil {
		return "main"
	}
	id := []rune(strings.TrimPrefix(printable(a.ID), "agent-"))
	if len(id) > 4 {
		id = id[:4]
	}
	typ := printable(a.Type)
	if typ == "" {
		typ = "subagent"
	}
	return typ + "·" + string(id)
}

// printable drops every rune that is not graphic: control bytes, escape
// sequences' introducers, and format characters such as bidi overrides.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) {
			return r
		}
		return -1
	}, s)
}

func timelineResult(c TimelineCall) string {
	var r string
	switch c.Group {
	case GroupFailed:
		r = "failed"
		if c.ExitCode != nil {
			r = fmt.Sprintf("failed (exit %d)", *c.ExitCode)
		}
	default:
		r = c.Outcome
	}
	if c.Seq == nil {
		r += ", no declaration recorded"
	}
	return r
}

// laterLabel says how a failed call was followed up. Only failed calls get
// one, so an ok, denied or unknown row never carries an arrow.
//
// "No later success RECORDED": the record is all this can read, and a session
// with a gap in its coverage or a paused stretch can hold a success it never
// wrote down. "Of the same command or program": that is all laterSuccess looks
// for, and for a program with no same-program tier -- `go`, say -- only "of the
// same command", since a failed `go vet` followed by a passing `go test` has a
// later success recorded, just not one this matches.
func laterLabel(c TimelineCall) string {
	if c.Group != GroupFailed {
		return ""
	}
	if !c.LaterChecked {
		return "  → not checked for a later success"
	}
	l := c.Later
	if l == nil {
		if !sameProgramTier(c.Program) {
			return "  → no later success of the same command recorded"
		}
		return "  → no later success of the same command or program recorded"
	}
	who := ""
	if l.Agent != nil {
		who = ", " + agentLabel(l.Agent)
	}
	if l.Kind == LaterSameCommand {
		return fmt.Sprintf("  → same command ok at %d, recorded after%s", l.Seq, who)
	}
	// No claim about the arguments: a different digest is only a different
	// line. See LaterSameProgram.
	return fmt.Sprintf("  → same program ok at %d, recorded after%s", l.Seq, who)
}
