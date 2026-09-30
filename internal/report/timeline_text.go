package report

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// timelineRows caps the text listing. Past it the rest is counted, never
// silently cut; JSON always carries every call.
const timelineRows = 500

// writeTimeline renders the timeline: the counts first, so a reader sees what
// failed and what never ran before scrolling, then one row per call.
func writeTimeline(b *bytes.Buffer, t Timeline) {
	n := t.Counts
	fmt.Fprintf(b, "  timeline: %d call%s, main agent + %d subagent%s, in the order they were recorded\n",
		n.Calls, plural(n.Calls), n.Subagents, plural(n.Subagents))
	if n.Calls == 0 {
		return
	}
	fmt.Fprintf(b, "    ok           %d\n", n.OK)
	fmt.Fprintf(b, "    failed       %d  (%d same command succeeded later, %d same program succeeded later, %d no later success)\n",
		n.Failed, n.SameCommand, n.SameProgram, n.NoLater)
	if n.Interrupted > 0 {
		fmt.Fprintf(b, "    interrupted  %d\n", n.Interrupted)
	}
	fmt.Fprintf(b, "    never ran    %d  (denied before running)\n", n.NeverRan)
	fmt.Fprintf(b, "    unknown      %d  (no execution record: denied, failed or unrecorded, and the record cannot say which)\n", n.Unknown)
	if n.Subagents > 0 {
		fmt.Fprintln(b, "    calls from agents running at once interleave by when each was recorded, not when it started")
	}

	for i, c := range t.Calls {
		if i == timelineRows {
			rest := len(t.Calls) - timelineRows
			fmt.Fprintf(b, "    %d more call%s, see --json\n", rest, plural(rest))
			break
		}
		writeTimelineCall(b, c)
	}
}

func writeTimelineCall(b *bytes.Buffer, c TimelineCall) {
	seq, at := "-", "--:--:--"
	if c.Seq != nil {
		seq = fmt.Sprint(*c.Seq)
	}
	if c.RecordedAtMS != nil {
		at = time.UnixMilli(*c.RecordedAtMS).UTC().Format("15:04:05")
	}
	call := c.ToolName
	if c.Program != "" {
		call += " " + c.Program
	}
	fmt.Fprintf(b, "    %5s  %s  %-22s %-22s %s%s\n",
		seq, at, agentLabel(c.Agent), call, timelineResult(c), laterLabel(c))
	if l := bendingLabel(c.Bending); l != "" {
		fmt.Fprintf(b, "           %s\n", l)
	}
}

// bendingLabel annotates the row that completes a test-bending pair, on a line
// of its own under it. It says what the record shows between the two runs and
// nothing about why: "no recorded file edit", never "nothing changed", because a shell
// command between them could have changed files the record does not see.
func bendingLabel(t *TimelineBending) string {
	if t == nil {
		return ""
	}
	if t.Kind == BendTestsOnlyThenGreen {
		// ↳ DOWNWARDS ARROW WITH TIP RIGHTWARDS
		return fmt.Sprintf("↳ only files named like tests edited since %d, where the same command failed", t.Since)
	}
	return fmt.Sprintf("↳ same command had the other outcome at %d, no recorded file edit between", t.Since)
}

// agentLabel names the agent a call ran in: "main", or the subagent's type
// with the first four characters of its id, which keeps two subagents of one
// type apart without printing ids a reader cannot use. An "agent-" prefix is
// dropped first, or every label would read "agen".
func agentLabel(a *TimelineAgent) string {
	if a == nil {
		return "main"
	}
	id := strings.TrimPrefix(a.ID, "agent-")
	if len(id) > 4 {
		id = id[:4]
	}
	typ := a.Type
	if typ == "" {
		typ = "subagent"
	}
	return typ + "·" + id
}

func timelineResult(c TimelineCall) string {
	switch c.Group {
	case GroupFailed:
		if c.ExitCode != nil {
			return fmt.Sprintf("failed (exit %d)", *c.ExitCode)
		}
		return "failed"
	case GroupUnknown:
		if c.Outcome == LinkOutcomeNoRecord || c.Outcome == LinkOutcomeUnobserved {
			return c.Outcome
		}
		return "no declaration recorded"
	default:
		return c.Outcome
	}
}

// laterLabel says how a failed call was followed up. Only failed calls get
// one, so an ok, denied or unknown row never carries an arrow.
func laterLabel(c TimelineCall) string {
	if c.Group != GroupFailed {
		return ""
	}
	l := c.Later
	if l == nil {
		return "  → no later success"
	}
	who := ""
	if l.Agent != nil {
		who = ", " + agentLabel(l.Agent)
	}
	if l.Kind == LaterSameCommand {
		return fmt.Sprintf("  → same command ok at %d%s", l.Seq, who)
	}
	return fmt.Sprintf("  → same program ok at %d (different arguments%s)", l.Seq, who)
}
