package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Timeline groups. Four, and never merged: a call that ran and did not
// succeed, a call refused before it started, and a call whose ending the
// record cannot state are three different facts, and the whole point of the
// view is that a reader can tell them apart without opening a transcript.
const (
	GroupOK          = "ok"
	GroupFailed      = "failed"
	GroupInterrupted = "interrupted"
	GroupNeverRan    = "never ran"
	// GroupUnknown holds "no execution record", "outcome unobserved" and
	// dropped calls. "No execution record" is NOT never-ran: the store's own
	// contract is that such a declaration was denied, failed, or had its
	// execution go unrecorded, and nothing here knows which.
	GroupUnknown = "unknown"
)

// How a failed call was followed up, strongest first.
const (
	// LaterSameCommand: a later call with the same tool and the same shape
	// digest succeeded. The digest is an HMAC over the exact command line
	// (Bash) or the whole input, so equal digests are an identical call.
	LaterSameCommand = "same_command"
	// LaterSameProgram: a later call of the same tool and program, with a
	// different digest, succeeded. Weaker, and rendered as such.
	LaterSameProgram = "same_program"
)

// TimelineAgent is the subagent a call ran in. Nil on a call the main agent
// made, which is a fact about the payload rather than a missing value: only a
// subagent call carries agent_id.
type TimelineAgent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// LaterSuccess points at the first later call that succeeded where this one
// failed. It says the same call, or the same program, succeeded afterwards --
// never that anything was fixed: a re-run that passes proves only that it
// passed the second time.
type LaterSuccess struct {
	Kind  string         `json:"kind"`
	Seq   int64          `json:"seq"`
	Agent *TimelineAgent `json:"agent"`
}

// TimelineCall is one call, main agent or subagent, in the session's order.
type TimelineCall struct {
	// Seq and RecordedAtMS are null on a dropped call: its declaration never
	// landed, so it has no position in the ordered stream and no clock.
	Seq          *int64         `json:"seq"`
	RecordedAtMS *int64         `json:"recorded_at_unix_ms"`
	ToolUseID    string         `json:"tool_use_id"`
	Agent        *TimelineAgent `json:"agent"`
	ToolName     string         `json:"tool_name"`
	Program      string         `json:"program,omitempty"`
	VerbClass    string         `json:"verb_class"`
	Group        string         `json:"group"`
	Outcome      string         `json:"outcome"`
	ExitCode     *int           `json:"exit_code"`
	Later        *LaterSuccess  `json:"later"`
	// Bending is set on the LATER call of a test-bending pair (see
	// DetectTestBending): the run that passed after only files named like
	// tests were edited, or the run whose outcome differs from the same
	// command's previous run with no recorded file edit between. Null on
	// every other row.
	Bending *TimelineBending `json:"test_bending"`
}

// TimelineBending names the pattern a row completes and the earlier run of the
// same command it pairs with.
type TimelineBending struct {
	Kind  string `json:"kind"`
	Since int64  `json:"since_seq"`
}

// TimelineCounts is the timeline's summary, per group, with the failed calls
// split by how they were followed up.
type TimelineCounts struct {
	Calls       int `json:"calls"`
	Subagents   int `json:"subagents"`
	OK          int `json:"ok"`
	Failed      int `json:"failed"`
	SameCommand int `json:"failed_later_same_command"`
	SameProgram int `json:"failed_later_same_program"`
	NoLater     int `json:"failed_no_later_success"`
	Interrupted int `json:"interrupted"`
	NeverRan    int `json:"never_ran"`
	Unknown     int `json:"unknown"`
}

// Timeline is every call the session made, main agent and subagents on one
// list, in seq order.
//
// Seq is allocated under the run's append lock, so it is one total order over
// every agent's calls. That order is when each call's hook RECORDED, which for
// two subagents running at once interleaves them by recording, not by start;
// the renderer says so.
type Timeline struct {
	Calls  []TimelineCall `json:"calls"`
	Counts TimelineCounts `json:"counts"`
}

type timelineEntry struct {
	call   TimelineCall
	digest string
}

func buildTimeline(run *store.Run, denied map[string]bool) Timeline {
	out := Timeline{Calls: []TimelineCall{}}
	if run == nil {
		return out
	}
	executed := executionsByID(run)

	decls := append([]store.Declaration(nil), run.Declarations...)
	sort.SliceStable(decls, func(i, j int) bool { return decls[i].Seq < decls[j].Seq })

	entries := make([]timelineEntry, 0, len(decls))
	agents := map[string]bool{}
	for _, d := range decls {
		seq, at := d.Seq, d.RecordedAtMS
		c := TimelineCall{
			Seq:          &seq,
			RecordedAtMS: &at,
			ToolUseID:    d.ToolUseID,
			Agent:        timelineAgent(d),
			ToolName:     d.ToolName,
			VerbClass:    d.Shape.VerbClass,
		}
		if d.Shape.Program != nil {
			c.Program = *d.Shape.Program
		}
		if c.Agent != nil {
			agents[c.Agent.ID] = true
		}
		c.Outcome, _, _ = linkOutcome(d.ToolUseID, executed, denied)
		c.Group = timelineGroup(c.Outcome)
		c.ExitCode = lastExitCode(executed[d.ToolUseID])
		entries = append(entries, timelineEntry{call: c, digest: d.Shape.Digest})
	}

	for i := range entries {
		if entries[i].call.Group == GroupFailed {
			entries[i].call.Later = laterSuccess(entries, i)
		}
	}

	// A seq is one call, so the later seq of a pair names its row.
	bending := map[int64]TimelineBending{}
	tb := DetectTestBending(run, denied)
	for _, p := range tb.TestsOnlyThenGreen {
		bending[p[1]] = TimelineBending{Kind: BendTestsOnlyThenGreen, Since: p[0]}
	}
	for _, p := range tb.Flaky {
		bending[p[1]] = TimelineBending{Kind: BendFlaky, Since: p[0]}
	}
	for i := range entries {
		if b, ok := bending[*entries[i].call.Seq]; ok {
			entries[i].call.Bending = &b
		}
	}

	for _, e := range entries {
		out.Calls = append(out.Calls, e.call)
	}
	for _, id := range run.Dropped() {
		out.Calls = append(out.Calls, TimelineCall{
			ToolUseID: id,
			ToolName:  LinkUnknown,
			VerbClass: LinkUnknown,
			Group:     GroupUnknown,
			Outcome:   LinkUnknown,
		})
	}
	out.Counts = countTimeline(out.Calls, len(agents))
	return out
}

func timelineAgent(d store.Declaration) *TimelineAgent {
	if d.AgentID == nil || *d.AgentID == "" {
		return nil
	}
	a := &TimelineAgent{ID: *d.AgentID}
	if d.AgentType != nil {
		a.Type = *d.AgentType
	}
	return a
}

func timelineGroup(outcome string) string {
	switch outcome {
	case store.ExecOK:
		return GroupOK
	case store.ExecFailed:
		return GroupFailed
	case store.ExecInterrupted:
		return GroupInterrupted
	case LinkOutcomeDenied:
		return GroupNeverRan
	default:
		return GroupUnknown
	}
}

// lastExitCode is the exit code of the record linkOutcome took the outcome
// from: the highest seq, since executionsByID sorts ascending.
func lastExitCode(recs []store.Execution) *int {
	if len(recs) == 0 {
		return nil
	}
	return recs[len(recs)-1].ExitCode
}

// laterSuccess looks FORWARD from the failed call at i for the first success
// of the same command, and failing that the first success of the same
// program. Any agent's success counts: a subagent re-running what the main
// agent failed is the common case, and the result names which agent it was.
func laterSuccess(entries []timelineEntry, i int) *LaterSuccess {
	failed := entries[i]
	var sameProgram *LaterSuccess
	for j := i + 1; j < len(entries); j++ {
		c := entries[j].call
		if c.Group != GroupOK || c.ToolName != failed.call.ToolName {
			continue
		}
		if failed.digest != "" && entries[j].digest == failed.digest {
			return &LaterSuccess{Kind: LaterSameCommand, Seq: *c.Seq, Agent: c.Agent}
		}
		if sameProgram == nil && failed.call.Program != "" && c.Program == failed.call.Program {
			sameProgram = &LaterSuccess{Kind: LaterSameProgram, Seq: *c.Seq, Agent: c.Agent}
		}
	}
	return sameProgram
}

func countTimeline(calls []TimelineCall, subagents int) TimelineCounts {
	n := TimelineCounts{Calls: len(calls), Subagents: subagents}
	for _, c := range calls {
		switch c.Group {
		case GroupOK:
			n.OK++
		case GroupFailed:
			n.Failed++
			switch {
			case c.Later == nil:
				n.NoLater++
			case c.Later.Kind == LaterSameCommand:
				n.SameCommand++
			default:
				n.SameProgram++
			}
		case GroupInterrupted:
			n.Interrupted++
		case GroupNeverRan:
			n.NeverRan++
		default:
			n.Unknown++
		}
	}
	return n
}
