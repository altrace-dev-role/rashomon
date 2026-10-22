package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// LinkUnknown is the value of a field nothing in the record can fill: a
// dropped declaration's tool, verb and outcome, where the id is all there is.
const LinkUnknown = "unknown"

// How one link ended. store.ExecOK, store.ExecFailed and store.ExecInterrupted
// are the recorded outcomes; these four are the cases where there is no
// outcome to report, kept apart because each is a different fact.
const (
	// LinkOutcomeDenied: the call was refused before it ran -- by the user at
	// the permission prompt, by the auto mode classifier, or by a rule. The
	// wording names what is known, that it never ran, and not who refused it.
	// The product working, not the call failing.
	LinkOutcomeDenied = "denied before running"
	// LinkOutcomeUnobserved: an execution record exists with no outcome -- a v1
	// record read back. The call ran; how it ended was never written down.
	LinkOutcomeUnobserved = "outcome unobserved"
	// LinkOutcomeBackgrounded: the execution record was written while the
	// command was still running in the background (store.Execution's
	// Backgrounded) -- moved there at its timeout, or launched with
	// run_in_background, or on Ctrl+B if Claude Code marks that the same way
	// (not measured). Its recorded ok is the launch's; how the command
	// itself ended was never recorded, so it reads as unobserved, never as a
	// success.
	LinkOutcomeBackgrounded = "moved to the background, outcome unobserved"
	// LinkOutcomeNoRecord: no execution record at all. The store's own comment
	// is the reason this is its own value: such a declaration was denied,
	// failed, or had its execution go unrecorded, and nothing in the record
	// knows which. Naming it "failed" would pick one and assert it.
	LinkOutcomeNoRecord = "no execution record"
	// LinkOutcomeStatusMasked: the call recorded ok, and its shape says a
	// build or test runner on its line has an exit status the line does not
	// return (shape.status_masked): `make test 2>&1 | tail -40`, `go test
	// ./... ; echo $?`, `npm test || true`, `if make test; then echo ok;
	// fi`, `make test &`. The ok is the last command's.
	// The runner's own status was never recorded, so it reads with the
	// unobserved outcomes and never as a success. Timeline and test runs
	// only: the chains keep the call's own outcome.
	LinkOutcomeStatusMasked = "ok, but the line did not return a build or test run's exit status"
)

// Link is one tool call, in the position seq gives it.
type Link struct {
	Seq       int64  `json:"seq"`
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	Program   string `json:"program,omitempty"`
	VerbClass string `json:"verb_class"`
	// Outcome comes from the execution record with the HIGHER SEQ when there
	// are two -- a PostToolUse and a PostToolUseFailure can both write for one
	// id. Outcomes lists both, and ExecutionRecords counts them, because
	// picking one and showing only it is how the failure becomes the half that
	// disappears.
	Outcome          string   `json:"outcome"`
	Outcomes         []string `json:"outcomes"`
	ExecutionRecords int      `json:"execution_records"`
	// DeclaredHosts is what this call NAMED, never what it reached: nothing
	// in the record observed whether it got there. The renderer says so in
	// its legend, because a per-call reading is the obvious misreading and it
	// is the kind that overstates what is known. Named as the destinations
	// list is, and renamed from `hosts` rather than retyped, so a consumer
	// that read the old per-host rows fails on the missing key.
	DeclaredHosts []string `json:"declared_hosts"`
	// SSHHosts is kept APART from DeclaredHosts because an ssh host is a
	// different kind of destination from an HTTPS one, and the family view
	// treats it so.
	SSHHosts []string `json:"ssh_hosts"`
}

// Chain is one prompt and the calls it produced.
type Chain struct {
	TranscriptPath string `json:"transcript_path"`
	PromptID       string `json:"prompt_id"`
	Links          []Link `json:"links"`
}

// Chains is the causal view of a session.
//
// It carries no prompt TEXT. The store records a prompt id, not a prompt, and
// reading the wording out of the transcript would open a third content path
// through this package for a display convenience. The id joins to the
// transcript the report already names, so the text is one grep away for the one
// person who has it; that is the right side to err on for a tool whose store
// holds no content by construction.
type Chains struct {
	Prompts []Chain `json:"prompts"`
	// Unattributed holds declarations carrying no prompt id -- every v1
	// record, and a subagent call whose payload never carried one. They cannot
	// be placed under a prompt, so they are placed HERE, as links with ids,
	// rather than counted: a count says how much is missing and a set equality
	// check needs to know WHICH, and only the second can prove nothing
	// vanished.
	Unattributed []Link `json:"unattributed"`
	// Dropped is one link per terminal record whose declaration never landed.
	// Every field but the id is unknown, because every field but the id is
	// genuinely unknown -- but the call happened, and a view that omitted it
	// would be a complete-looking account of a session with a hole in it.
	Dropped []Link `json:"dropped"`
}

// deniedSet collects the ids the transcripts say the user refused.
//
// Read from the transcript rather than from the store because a denial leaves
// no record here to read: the call never ran, so there is no execution, and the
// declaration alone cannot say why. The transcript's tool_result is the only
// place the refusal is written down.
func deniedSet(ts []Transcript) map[string]bool {
	out := map[string]bool{}
	for _, t := range ts {
		for _, id := range t.DeniedByUser {
			out[id] = true
		}
	}
	return out
}

// buildChains assembles the prompt-to-call spine.
//
// executed is run's executions grouped by tool_use_id (executionsByID), which
// Build groups once for the chains, the timeline and the test runs. forgotten
// is applied to each link's own host lists against that link's own
// declaration time, the same per-declaration rule buildDestinations applies.
func buildChains(run *store.Run, executed map[string][]store.Execution, denied map[string]bool, forgotten func(host string, declaredAtMS int64) bool) Chains {
	out := Chains{Prompts: []Chain{}, Unattributed: []Link{}, Dropped: []Link{}}
	if run == nil {
		return out
	}

	type key struct{ transcript, prompt string }
	byKey := map[key]*Chain{}
	first := map[key]int64{}
	var order []key

	for _, d := range run.Declarations {
		if d.PromptID == nil || *d.PromptID == "" {
			out.Unattributed = append(out.Unattributed,
				buildLink(d, executed, denied, forgotten))
			continue
		}
		k := key{transcript: d.TranscriptPath, prompt: *d.PromptID}
		c, seen := byKey[k]
		if !seen {
			c = &Chain{TranscriptPath: d.TranscriptPath, PromptID: *d.PromptID}
			byKey[k] = c
			first[k] = d.Seq
			order = append(order, k)
		}
		if d.Seq < first[k] {
			first[k] = d.Seq
		}
		c.Links = append(c.Links, buildLink(d, executed, denied, forgotten))
	}

	// Terminals whose declaration never landed. The id is all there is, and
	// saying so is the point: these are calls the run made and cannot describe.
	for _, id := range run.Dropped() {
		out.Dropped = append(out.Dropped, Link{
			ToolUseID:     id,
			ToolName:      LinkUnknown,
			VerbClass:     LinkUnknown,
			Outcome:       LinkUnknown,
			Outcomes:      []string{},
			DeclaredHosts: []string{},
			SSHHosts:      []string{},
		})
	}
	sort.SliceStable(out.Dropped, func(i, j int) bool {
		return out.Dropped[i].ToolUseID < out.Dropped[j].ToolUseID
	})
	sort.SliceStable(out.Unattributed, func(i, j int) bool {
		return out.Unattributed[i].Seq < out.Unattributed[j].Seq
	})

	// Chains by where they start, links by seq. Both are the same claim -- that
	// this is the order things happened in -- and it is the only claim of that
	// kind the report makes.
	sort.SliceStable(order, func(i, j int) bool { return first[order[i]] < first[order[j]] })
	for _, k := range order {
		c := byKey[k]
		sort.SliceStable(c.Links, func(i, j int) bool { return c.Links[i].Seq < c.Links[j].Seq })
		out.Prompts = append(out.Prompts, *c)
	}
	return out
}

// executionsByID groups the run's execution records by tool_use_id.
//
// A single id can have TWO: PostToolUse and PostToolUseFailure both write, and
// the previous version of this code kept a bare map assigned in slice order, so
// the last record read won silently -- which in the two-record case is a coin
// toss that can discard the failure. Slice order is not a safe proxy for seq
// either: Execution.Seq is a nullable pointer, because a record written to the
// spill file when the ordered stream's lock could not be taken lands without a
// position.
func executionsByID(run *store.Run) map[string][]store.Execution {
	// The bucket count, not the total: most ids hold one record and a few
	// hold two, so this undercounts buckets slightly rather than guessing,
	// but it is never wrong in the expensive direction -- run.Executions is
	// already read and real, not a size hint over something unread.
	out := make(map[string][]store.Execution, len(run.Executions))
	for _, e := range run.Executions {
		out[e.ToolUseID] = append(out[e.ToolUseID], e)
	}
	for id := range out {
		recs := out[id]
		// Highest seq last. A record with no seq sorts BELOW one that has a
		// position, never above it: it is the record whose order nobody knows,
		// and letting an unknown position outrank a known one is how the pick
		// rule becomes arbitrary again by another route.
		sort.SliceStable(recs, func(i, j int) bool {
			return execSeq(recs[i]) < execSeq(recs[j])
		})
		out[id] = recs
	}
	return out
}

func execSeq(e store.Execution) int64 {
	if e.Seq == nil {
		return -1
	}
	return *e.Seq
}

func buildLink(d store.Declaration, executed map[string][]store.Execution, denied map[string]bool, forgotten func(host string, declaredAtMS int64) bool) Link {
	l := Link{
		Seq:           d.Seq,
		ToolUseID:     d.ToolUseID,
		ToolName:      d.ToolName,
		VerbClass:     d.Shape.VerbClass,
		DeclaredHosts: []string{},
		SSHHosts:      []string{},
	}
	if d.Shape.Program != nil {
		l.Program = *d.Shape.Program
	}
	l.Outcome, l.Outcomes, l.ExecutionRecords = linkOutcome(d.ToolUseID, executed, denied)

	// Carried as named. Nothing in the record observed whether a call reached
	// a host, and a column that read as a verdict would be answering a
	// question nothing was asked. forgotten drops a host this declaration
	// named at or before the latest forget --host of it -- stale evidence a
	// failed removal left behind -- and keeps one named after, which is this
	// call's own current information.
	for _, h := range d.Hosts {
		if forgotten != nil && forgotten(h, d.RecordedAtMS) {
			continue
		}
		l.DeclaredHosts = append(l.DeclaredHosts, h)
	}
	sort.Strings(l.DeclaredHosts)
	for _, h := range d.SSHHosts {
		if forgotten != nil && forgotten(h, d.RecordedAtMS) {
			continue
		}
		l.SSHHosts = append(l.SSHHosts, h)
	}
	sort.Strings(l.SSHHosts)
	return l
}

// linkOutcome returns the headline outcome, every outcome recorded for the id,
// and how many records there were.
//
// All three, because one number and one word answer different questions and a
// reader given only the word cannot tell a single clean result from a pair
// that disagreed.
func linkOutcome(id string, executed map[string][]store.Execution, denied map[string]bool) (string, []string, int) {
	recs, ok := executed[id]
	if !ok || len(recs) == 0 {
		// Checked only when no execution record exists. A denial produces no
		// execution record, so the two cannot both be true -- and were a future
		// change to make them so, the record would be the stronger evidence.
		if denied[id] {
			return LinkOutcomeDenied, []string{}, 0
		}
		return LinkOutcomeNoRecord, []string{}, 0
	}

	all := make([]string, 0, len(recs))
	for _, e := range recs {
		o := e.Outcome
		switch {
		case e.Backgrounded:
			o = LinkOutcomeBackgrounded
		case o == "":
			o = LinkOutcomeUnobserved
		}
		all = append(all, o)
	}
	// The LAST record after the seq sort: the highest position wins. Both are
	// listed above it either way, so the pick decides emphasis and not what the
	// reader is allowed to see.
	return all[len(all)-1], all, len(recs)
}
