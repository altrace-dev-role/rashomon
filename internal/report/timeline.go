package report

import (
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Timeline groups. Five, and never merged: a call that ran and did not
// succeed, a call refused before it started, and a call whose ending the
// record cannot state are three different facts, and the whole point of the
// view is that a reader can tell them apart without opening a transcript.
// Snake case, as later.kind and the counts' keys are.
const (
	GroupOK          = "ok"
	GroupFailed      = "failed"
	GroupInterrupted = "interrupted"
	GroupNeverRan    = "never_ran"
	// GroupUnknown holds "no execution record", "outcome unobserved" and a
	// call moved to the background before it ended. "No
	// execution record" is NOT never-ran: the store's own contract is that such
	// a declaration was denied, failed, or had its execution go unrecorded, and
	// nothing here knows which. A call whose declaration was dropped is not
	// here by virtue of the drop: an execution record it left says how it
	// ended, and it goes to that record's group.
	GroupUnknown = "unknown"
)

// How a failed call was followed up, strongest first.
const (
	// LaterSameCommand: a later call with the same tool and the same effective
	// digest succeeded. The digest is an HMAC over the exact command line
	// (Bash) or the whole input, so equal digests are an identical call. The
	// effective digest is the one the call RAN with -- its outcome record's
	// executed digest -- and the declared one only where that record carries
	// none: a PreToolUse hook can rewrite the input, and a success that ran
	// something other than what was declared is not a re-run of the failure.
	LaterSameCommand = "same_command"
	// LaterSameProgram: a later call of the same tool and program, with a
	// different digest, succeeded. Weaker, and rendered as nothing more: a
	// different digest is a different command LINE, which `CI=1 pytest -q`,
	// `cd sub && pytest -q` and `pytest  -q` all are next to `pytest -q`, so it
	// says nothing about the arguments. Never offered for a program in
	// subcommandPrograms, or a versioned name of one. Nor where either call
	// was rewritten: the program is the declared one, the record keeps no
	// executed program, and a rewrite can change it -- such a success leaves
	// the failure not checked.
	LaterSameProgram = "same_program"
)

// subcommandPrograms are programs whose next word, not the program, names what
// ran: `git status` succeeding says nothing about a failed `git push`, nor
// `python b.py` about `python a.py`, and the same-program tier would pair
// them. Wrappers and launchers are here for the same reason: the program is
// the first word, so `sudo ls` would otherwise follow up a failed `sudo
// systemctl restart`, `ssh prod uptime` a failed `ssh prod systemctl restart
// nginx`, and `timeout 5 true` a failed `timeout 60 go test ./...`. The
// same-command tier is still offered, because an identical digest is an
// identical line whatever the program. A closed list and a short one: a
// program missing from it gets the weak tier, which the text renders as no
// more than "same program".
var subcommandPrograms = map[string]bool{
	"git": true, "gh": true, "go": true, "cargo": true, "make": true,
	"npm": true, "npx": true, "pnpm": true, "yarn": true, "bun": true, "deno": true,
	"pip": true, "pip3": true, "uv": true, "poetry": true,
	"docker": true, "podman": true, "kubectl": true, "helm": true, "terraform": true,
	"aws": true, "gcloud": true, "az": true, "dotnet": true, "mvn": true, "gradle": true,
	"brew": true, "apt": true, "apt-get": true, "systemctl": true,
	"python": true, "python3": true, "node": true, "nodejs": true, "ruby": true, "perl": true,
	"bash": true, "sh": true, "zsh": true,
	"pipx": true, "uvx": true, "bunx": true, "pnpx": true, "bundle": true, "pipenv": true,
	"conda": true, "nix": true, "direnv": true, "mise": true,
	"sudo": true, "doas": true, "su": true, "runuser": true, "chroot": true, "ssh": true,
	"env": true, "timeout": true, "gtimeout": true, "time": true, "gtime": true, "nohup": true,
	"nice": true, "ionice": true, "setsid": true, "flock": true, "strace": true, "parallel": true,
	"xargs": true, "watch": true, "stdbuf": true, "unbuffer": true, "xvfb-run": true,
	"exec": true, "command": true,
}

// subcommandProgram says whether a program is in subcommandPrograms, directly
// or as a suffixed name of one: python3.12, python3.13t, pip3.11, node18,
// python.exe and pythonw are the interpreter they name. So the name is looked
// up with a trailing .exe dropped and cut at its first digit, and then with
// one trailing w dropped as well.
func subcommandProgram(p string) bool {
	if subcommandPrograms[p] {
		return true
	}
	base := strings.TrimSuffix(p, ".exe")
	if i := strings.IndexAny(base, "0123456789"); i >= 0 {
		base = base[:i]
	}
	return subcommandPrograms[base] || subcommandPrograms[strings.TrimSuffix(base, "w")]
}

// sameProgramTier says whether a failure of this program is followed up by a
// success of the same program as well as of the same command: not when the
// call has no program, nor for one whose next word names what ran.
func sameProgramTier(program string) bool {
	return program != "" && !subcommandProgram(program)
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
//
// A call moved to the background is never the later success. Its record is
// written at the launch, not at the command's end, so its "ok" says only that
// it started; that is `run_in_background: true`, and also a command Claude
// Code moved to the background when it reached its timeout (unless it starts
// with `sleep`) or when the user pressed Ctrl+B. From schema 3 the execution
// record says so (store.Execution's Backgrounded; on Ctrl+B only if Claude
// Code marks it with backgroundTaskId or backgroundedByUser, which was not
// measured), and such a call is in the unknown group, which laterSuccess does
// not look in. A record written
// before schema 3 cannot say, and its launch still reads as ok.
type LaterSuccess struct {
	Kind  string         `json:"kind"`
	Seq   int64          `json:"seq"`
	Agent *TimelineAgent `json:"agent"`
}

// TimelineCall is one call, main agent or subagent, in the session's order.
type TimelineCall struct {
	// Seq and RecordedAtMS are the declaration's, and null on a call with no
	// declaration: its execution record may have a position, but the call has
	// no declaration position and no declaration clock.
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
	// where nothing placed was found but a success not recorded before it
	// cannot be ruled out: a matching success whose record has no seq, a
	// same-program success where either call was rewritten, a matching ok
	// record of a failed call, or a success whose declaration was lost and
	// whose executed digest does not rule it out (any of them may be the
	// later one). A nil Later there is "not checked", never "no later
	// success".
	LaterChecked bool `json:"later_checked"`
	// Bending is set on the LATER call of a test-bending pair (see
	// DetectTestBending): the run that passed when the only recorded edits
	// since the same command failed were to files named like tests, or the
	// run whose outcome differs from the same command's previous run with no
	// recorded file edit between. Null on every other row.
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
// the ordered ones, by the seq of the record each outcome is read from (the
// last failed one, otherwise the last).
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
	// rewritten is true when the outcome record ran another digest than the
	// declared one: a PreToolUse hook rewrote the input, so the declared
	// program may not be the one that ran.
	rewritten bool
	// ok is a failed call's last ok record, which its row does not show: a
	// success of the call, recorded where it was. Nil on any other call.
	ok *timelineOK
}

// timelineOK is the success a failed call's ok record holds, as laterSuccess
// weighs it: where it was recorded and what it ran.
type timelineOK struct {
	pos    *int64
	digest string
}

func buildTimeline(run *store.Run, denied map[string]bool) Timeline {
	if run == nil {
		return timelineFrom(nil, nil, denied, TestBending{})
	}
	executed := executionsByID(run)
	return timelineFrom(run, executed, denied, detectTestBending(run, executed, denied))
}

// timelineFrom is buildTimeline over run's executions already grouped
// (executionsByID) and the test-bending pairs already found over them: Build
// computes both once and shares them with the test runs.
func timelineFrom(run *store.Run, executed map[string][]store.Execution, denied map[string]bool, tb TestBending) Timeline {
	out := Timeline{Calls: []TimelineCall{}}
	if run == nil {
		return out
	}

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
		rec := outcomeRecord(executed[d.ToolUseID])
		c.Outcome = timelineOutcome(d.ToolUseID, rec, executed, denied)
		c.Group = timelineGroup(c.Outcome)
		c.ExitCode = outcomeExitCode(rec)
		entries = append(entries, timelineEntry{
			call:      c,
			digest:    effectiveDigest(d.Shape.Digest, rec),
			pos:       outcomeSeq(rec),
			rewritten: rec != nil && rec.ExecutedDigest != "" && rec.ExecutedDigest != d.Shape.Digest,
			ok:        failedCallOK(c, d.Shape.Digest, executed[d.ToolUseID]),
		})
	}

	// Calls with no declaration: a terminal or an execution record names them
	// and nothing else does. The outcome is read as for any other call,
	// because an execution record that says failed is a failure whether or not
	// its declaration landed -- the report's own failed-calls count counts it,
	// and a timeline that did not would disagree with it on the same page. The
	// tool name is the execution's own; agent, program, and the declaration's
	// position and time are unknown and said to be.
	//
	// The two counts agree per call, not per record: a call any of whose
	// execution records failed is failed here, as it is there, but the report
	// counts every failed record, so two failed records on one call are two
	// there and one here.
	for _, id := range undeclared(run, executed) {
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
		rec := outcomeRecord(recs)
		c.Outcome = timelineOutcome(id, rec, executed, denied)
		c.Group = timelineGroup(c.Outcome)
		c.ExitCode = outcomeExitCode(rec)
		entries = append(entries, timelineEntry{call: c, digest: effectiveDigest("", rec), pos: outcomeSeq(rec), ok: failedCallOK(c, "", recs)})
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

	// A seq is one call, so the later seq of a pair names its row.
	bending := map[int64]TimelineBending{}
	for _, p := range tb.TestsOnlyThenGreen {
		bending[p[1]] = TimelineBending{Kind: BendTestsOnlyThenGreen, Since: p[0]}
	}
	for _, p := range tb.Flaky {
		bending[p.Seqs[1]] = TimelineBending{Kind: BendFlaky, Since: p.Seqs[0]}
	}
	for i := range entries {
		// A row with no declaration has no seq, and no pair can name it.
		if entries[i].call.Seq == nil {
			continue
		}
		if b, ok := bending[*entries[i].call.Seq]; ok {
			entries[i].call.Bending = &b
		}
	}

	for _, e := range entries {
		out.Calls = append(out.Calls, e.call)
	}
	out.Counts = countTimeline(out.Calls, len(agents))
	return out
}

// undeclared returns the ids a terminal or an execution record names and no
// declaration does, once each. Dropped() alone misses an execution record
// with no terminal either.
//
// Ordered by the seq of the execution record each one's outcome is read from,
// so they read in the order their results were written. Those with no
// positioned record come last, and ties go by id.
func undeclared(run *store.Run, executed map[string][]store.Execution) []string {
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
	pos := func(id string) (int64, bool) {
		p := outcomeSeq(outcomeRecord(executed[id]))
		if p == nil {
			return 0, false
		}
		return *p, true
	}
	sort.Slice(ids, func(i, j int) bool {
		pi, oki := pos(ids[i])
		pj, okj := pos(ids[j])
		if oki != okj {
			return oki
		}
		if pi != pj {
			return pi < pj
		}
		return ids[i] < ids[j]
	})
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

// outcomeRecord is the execution record a call's outcome is read from, of
// one id's records sorted by seq (executionsByID sorts them): the highest-seq
// one that failed when any did, and otherwise the highest-seq one. Nil when
// there is none.
//
// A failure is never hidden behind a later record of the same id: the
// report's failed-calls count counts every failed record, and a row reading
// ok would put a failure on the page that the timeline does not show. The
// LAST failed record, because a success recorded between two failures of one
// call was followed by a failure, not after it.
func outcomeRecord(recs []store.Execution) *store.Execution {
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Outcome == store.ExecFailed {
			return &recs[i]
		}
	}
	if len(recs) == 0 {
		return nil
	}
	return &recs[len(recs)-1]
}

// timelineOutcome is the call's outcome: linkOutcome's, which reads the
// highest-seq record, unless the outcome record failed -- then failed.
func timelineOutcome(id string, rec *store.Execution, executed map[string][]store.Execution, denied map[string]bool) string {
	if rec != nil && rec.Outcome == store.ExecFailed {
		return store.ExecFailed
	}
	o, _, _ := linkOutcome(id, executed, denied)
	return o
}

// outcomeExitCode is the outcome record's exit code, nil when there is none.
func outcomeExitCode(rec *store.Execution) *int {
	if rec == nil {
		return nil
	}
	return rec.ExitCode
}

// effectiveDigest is the digest the call ran with: the outcome record's
// executed digest, or the declared one when the record carries none. An empty
// executed digest is "not known", never "different" -- see
// store.Execution.ExecutedDigest.
func effectiveDigest(declared string, rec *store.Execution) string {
	if rec != nil && rec.ExecutedDigest != "" {
		return rec.ExecutedDigest
	}
	return declared
}

// failedCallOK is the last ok record of a failed call, nil when the call is
// not failed or has none. The outcome is read from a failed record whenever
// one exists, so this success is on no row.
func failedCallOK(c TimelineCall, declared string, recs []store.Execution) *timelineOK {
	if c.Group != GroupFailed {
		return nil
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Outcome == store.ExecOK {
			return &timelineOK{pos: recs[i].Seq, digest: effectiveDigest(declared, &recs[i])}
		}
	}
	return nil
}

// outcomeSeq is the outcome record's seq, nil when there is none or it has none.
func outcomeSeq(rec *store.Execution) *int64 {
	if rec == nil {
		return nil
	}
	return rec.Seq
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
// success": the success is in the record and may well be the later one. A
// success of the same tool whose declaration was lost, recorded after the
// failure or at no known position, is the same unless its executed digest
// rules it out: it has no program and no row. So
// is a later success of the same program where either call was rewritten:
// nothing says which program ran. And so is the ok record of a failed call,
// which matches but has no ok row to point at.
func laterSuccess(entries []timelineEntry, i int) (*LaterSuccess, bool) {
	failed := entries[i]
	programTier := sameProgramTier(failed.call.Program)
	var sameCommand, sameProgram *timelineEntry
	unplaced := false
	for j := range entries {
		e := &entries[j]
		c := e.call
		// A success whose declaration was lost -- to a lock timeout, a paused
		// pre hook or a failing PreToolUse hook -- has no program to match and
		// no row to point at, but its execution record says what ran, and a
		// record with no tool name may be of the same tool. A known digest
		// other than the failure's rules it out where no program tier is
		// offered; anything else recorded after the failure, or at no known
		// position, is one more success that cannot be ruled out.
		if c.Seq == nil && c.Group == GroupOK {
			if c.ToolName != failed.call.ToolName && c.ToolName != LinkUnknown {
				continue
			}
			if e.digest != "" && e.digest != failed.digest && !programTier {
				continue
			}
			if e.pos == nil || *e.pos > *failed.pos {
				unplaced = true
			}
			continue
		}
		if c.ToolName != failed.call.ToolName {
			continue
		}
		// A failed call's ok record, this failure's own included: the same
		// command or program recorded after the failure, it is a success the
		// failure may be answered by, but its row says failed and is no row to
		// point at. It counts as one that cannot be ruled out, never as a match.
		if c.Group == GroupFailed {
			if ok := e.ok; ok != nil && (ok.pos == nil || *ok.pos > *failed.pos) {
				command := failed.digest != "" && ok.digest == failed.digest
				program := programTier && c.Program == failed.call.Program
				if command || program {
					unplaced = true
				}
			}
			continue
		}
		if c.Group != GroupOK {
			continue
		}
		command := failed.digest != "" && e.digest == failed.digest
		program := !command && programTier && c.Program == failed.call.Program
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
		// Either side rewritten, the declared programs say nothing about what
		// ran: a success that may be the later one, never one to point at.
		if program && (e.rewritten || failed.rewritten) {
			unplaced = true
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
