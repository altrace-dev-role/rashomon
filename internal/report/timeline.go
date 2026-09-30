package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Timeline groups. Five, and never merged: a call that ran and did not
// succeed, a call refused before it started, and a call whose ending the
// record cannot state are three different facts, and the whole point of the
// view is that a reader can tell them apart without opening a transcript.
const (
	GroupOK          = "ok"
	GroupFailed      = "failed"
	GroupInterrupted = "interrupted"
	GroupNeverRan    = "never ran"
	// GroupUnknown holds "no execution record" and "outcome unobserved". "No
	// execution record" is NOT never-ran: the store's own contract is that such
	// a declaration was denied, failed, or had its execution go unrecorded, and
	// nothing here knows which. A call whose declaration was dropped is not
	// here by virtue of the drop: an execution record it left says how it
	// ended, and it goes to that record's group.
	GroupUnknown = "unknown"
)

// How a failed call was followed up, strongest first.
const (
	// LaterSameCommand: a later call with the same tool and the same shape
	// digest succeeded. The digest is an HMAC over the exact command line
	// (Bash) or the whole input, so equal digests are an identical call.
	LaterSameCommand = "same_command"
	// LaterSameProgram: a later call of the same tool and program, with a
	// different digest, succeeded. Weaker, and rendered as nothing more: a
	// different digest is a different command LINE, which `CI=1 pytest -q`,
	// `cd sub && pytest -q` and `pytest  -q` all are next to `pytest -q`, so it
	// says nothing about the arguments. Never offered for a program in
	// subcommandPrograms.
	LaterSameProgram = "same_program"
)

// subcommandPrograms are programs whose next word, not the program, names what
// ran: `git status` succeeding says nothing about a failed `git push`, nor
// `python b.py` about `python a.py`, and the same-program tier would pair
// them. The same-command tier is still offered, because an identical digest
// is an identical line whatever the program. A closed list and a short one: a
// program missing from it gets the weak tier, which the text renders as no
// more than "same program".
var subcommandPrograms = map[string]bool{
	"git": true, "gh": true, "go": true, "cargo": true, "make": true,
	"npm": true, "npx": true, "pnpm": true, "yarn": true, "bun": true, "deno": true,
	"pip": true, "pip3": true, "uv": true, "poetry": true,
	"docker": true, "podman": true, "kubectl": true, "helm": true, "terraform": true,
	"aws": true, "gcloud": true, "az": true, "dotnet": true, "mvn": true, "gradle": true,
	"brew": true, "apt": true, "apt-get": true, "systemctl": true,
	"python": true, "python3": true, "node": true, "ruby": true, "perl": true,
	"bash": true, "sh": true, "zsh": true,
}

// TimelineAgent is the subagent a call ran in. Nil on a call the main agent
// made, which is a fact about the payload rather than a missing value: only a
// subagent call carries agent_id.
type TimelineAgent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// LaterSuccess points at the first call whose success was RECORDED AFTER this
// call's failure: its execution record's seq is the higher. Declaration order
// is the wrong clock for it -- two runs declared in one order can finish in
// the other, and a success that ended before the failure is not a later one.
// Seq is the success's row, so a reader can find it. It says the same call,
// or the same program, succeeded afterwards -- never that anything was fixed:
// a re-run that passes proves only that it passed the second time.
type LaterSuccess struct {
	Kind  string         `json:"kind"`
	Seq   int64          `json:"seq"`
	Agent *TimelineAgent `json:"agent"`
}

// TimelineCall is one call, main agent or subagent, in the session's order.
type TimelineCall struct {
	// Seq and RecordedAtMS are null on a call with no declaration: it has no
	// position in the ordered stream and no clock.
	Seq          *int64         `json:"seq"`
	RecordedAtMS *int64         `json:"recorded_at_unix_ms"`
	ToolUseID    string         `json:"tool_use_id"`
	Agent        *TimelineAgent `json:"agent"`
	// AgentUnknown is true on a call with no declaration. agent_id rides on
	// the declaration, so a nil Agent there would read as "the main agent"
	// when the record cannot say which agent it was.
	AgentUnknown bool          `json:"agent_unknown"`
	ToolName     string        `json:"tool_name"`
	Program      string        `json:"program,omitempty"`
	VerbClass    string        `json:"verb_class"`
	Group        string        `json:"group"`
	Outcome      string        `json:"outcome"`
	ExitCode     *int          `json:"exit_code"`
	Later        *LaterSuccess `json:"later"`
	// LaterChecked is true on a failed call that was compared against the
	// rest. False on a failed call with no declaration (no command to match),
	// whose failure record has no seq (no position to be later than), or
	// where nothing placed was found but a matching success's record has no
	// seq (it may be the later one): a nil Later there is "not checked",
	// never "no later success".
	LaterChecked bool `json:"later_checked"`
}

// TimelineCounts is the timeline's summary, per group, with the failed calls
// split by how they were followed up.
type TimelineCounts struct {
	Calls int `json:"calls"`
	// Who made the calls. Counted rather than assumed: a main agent that only
	// delegated made no calls of its own, and a header naming it beside the
	// subagents would say it had.
	MainAgent    int `json:"main_agent_calls"`
	Subagents    int `json:"subagents"`
	AgentUnknown int `json:"agent_unknown_calls"`
	OK           int `json:"ok"`
	Failed       int `json:"failed"`
	SameCommand  int `json:"failed_later_same_command"`
	SameProgram  int `json:"failed_later_same_program"`
	NoLater      int `json:"failed_no_later_success"`
	NotChecked   int `json:"failed_not_checked"`
	Interrupted  int `json:"interrupted"`
	NeverRan     int `json:"never_ran"`
	Unknown      int `json:"unknown"`
}

// Timeline is every call the session made, main agent and subagents on one
// list, in seq order.
//
// Seq is allocated under the run's append lock, so it is one total order over
// every agent's calls. That order is when each call's hook RECORDED, which for
// two subagents running at once interleaves them by recording, not by start;
// the renderer says so. Calls with no declaration have no seq and come after
// the ordered ones.
type Timeline struct {
	Calls  []TimelineCall `json:"calls"`
	Counts TimelineCounts `json:"counts"`
}

type timelineEntry struct {
	call   TimelineCall
	digest string
	// pos is the seq of the execution record the outcome was taken from: the
	// position a later success is measured against. Nil when there is no
	// record, or the record landed without a position.
	pos *int64
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
		recs := executed[d.ToolUseID]
		c.Outcome, _, _ = linkOutcome(d.ToolUseID, executed, denied)
		c.Group = timelineGroup(c.Outcome)
		c.ExitCode = lastExitCode(recs)
		entries = append(entries, timelineEntry{call: c, digest: d.Shape.Digest, pos: lastExecSeq(recs)})
	}

	// Calls with no declaration: a terminal or an execution record names them
	// and nothing else does. The outcome is read as for any other call,
	// because an execution record that says failed is a failure whether or not
	// its declaration landed -- the report's own failed-calls count counts it,
	// and a timeline that did not would disagree with it on the same page. The
	// tool name is the execution's own; agent, program and position are
	// unknown and said to be.
	for _, id := range undeclared(run) {
		recs := executed[id]
		c := TimelineCall{
			ToolUseID:    id,
			AgentUnknown: true,
			ToolName:     LinkUnknown,
			VerbClass:    LinkUnknown,
		}
		if len(recs) > 0 && recs[len(recs)-1].ToolName != "" {
			c.ToolName = recs[len(recs)-1].ToolName
		}
		c.Outcome, _, _ = linkOutcome(id, executed, denied)
		c.Group = timelineGroup(c.Outcome)
		c.ExitCode = lastExitCode(recs)
		entries = append(entries, timelineEntry{call: c, pos: lastExecSeq(recs)})
	}

	for i := range entries {
		e := &entries[i]
		if e.call.Group != GroupFailed {
			continue
		}
		// Not checked, rather than "no later success", when there is nothing
		// to check with: no declaration is no command to match, and no
		// position is no "after" to be.
		if e.call.Seq == nil || e.pos == nil {
			continue
		}
		e.call.Later, e.call.LaterChecked = laterSuccess(entries, i)
	}

	for _, e := range entries {
		out.Calls = append(out.Calls, e.call)
	}
	out.Counts = countTimeline(out.Calls, len(agents))
	return out
}

// undeclared returns the ids a terminal or an execution record names and no
// declaration does, once each and sorted, as the chains' dropped list is.
// Dropped() alone misses an execution record with no terminal either.
func undeclared(run *store.Run) []string {
	seen := map[string]bool{}
	for _, d := range run.Declarations {
		seen[d.ToolUseID] = true
	}
	var ids []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range run.Dropped() {
		add(id)
	}
	for _, x := range run.Executions {
		add(x.ToolUseID)
	}
	sort.Strings(ids)
	return ids
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

// lastExecSeq is that same record's seq, nil when it has none.
func lastExecSeq(recs []store.Execution) *int64 {
	if len(recs) == 0 {
		return nil
	}
	return recs[len(recs)-1].Seq
}

// laterSuccess finds, for the failed call at i, the success of the same
// command whose execution record was the first recorded after the failure's,
// and failing that the same for the same program. Every row is a candidate,
// whatever its declaration seq: completions cross, so the row order is not the
// order the results came in. Any agent's success counts: a subagent re-running
// what the main agent failed is the common case, and the result names which
// agent it was. The caller has checked the failure has a declaration and a
// position.
//
// The second result is whether the answer is one. A matching success whose
// record has no seq -- spilled when the append lock timed out, which is when
// agents run at once -- cannot be placed before or after the failure, so
// with nothing placed found the answer is "not checked", not "no later
// success": the success is in the record and may well be the later one.
func laterSuccess(entries []timelineEntry, i int) (*LaterSuccess, bool) {
	failed := entries[i]
	var sameCommand, sameProgram *timelineEntry
	unplaced := false
	for j := range entries {
		e := &entries[j]
		c := e.call
		if c.Group != GroupOK || c.ToolName != failed.call.ToolName {
			continue
		}
		command := failed.digest != "" && e.digest == failed.digest
		program := !command && failed.call.Program != "" && c.Program == failed.call.Program && !subcommandPrograms[c.Program]
		if !command && !program {
			continue
		}
		if e.pos == nil {
			unplaced = true
			continue
		}
		if *e.pos <= *failed.pos {
			continue
		}
		if command {
			if sameCommand == nil || *e.pos < *sameCommand.pos {
				sameCommand = e
			}
			continue
		}
		if sameProgram == nil || *e.pos < *sameProgram.pos {
			sameProgram = e
		}
	}
	switch {
	case sameCommand != nil:
		return &LaterSuccess{Kind: LaterSameCommand, Seq: *sameCommand.call.Seq, Agent: sameCommand.call.Agent}, true
	case sameProgram != nil:
		return &LaterSuccess{Kind: LaterSameProgram, Seq: *sameProgram.call.Seq, Agent: sameProgram.call.Agent}, true
	}
	return nil, !unplaced
}

func countTimeline(calls []TimelineCall, subagents int) TimelineCounts {
	n := TimelineCounts{Calls: len(calls), Subagents: subagents}
	for _, c := range calls {
		switch {
		case c.AgentUnknown:
			n.AgentUnknown++
		case c.Agent == nil:
			n.MainAgent++
		}
		switch c.Group {
		case GroupOK:
			n.OK++
		case GroupFailed:
			n.Failed++
			switch {
			case !c.LaterChecked:
				n.NotChecked++
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
