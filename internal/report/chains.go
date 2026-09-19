package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// The state of one host named by one link.
//
// LinkUnknown and LinkNotObserved are deliberately different values. "Not
// observed" says the wire was watched and this host never appeared, which is a
// finding. "Unknown" says the wire could not be attributed to this session at
// all, which is a coverage problem. Collapsing them turns a gap in the
// recorder into an accusation against the agent.
const (
	LinkReached   = "reached"
	LinkAttempted = "attempted"
	// LinkNotObserved: the call named this host and no row on the wire matches
	// it inside the window.
	LinkNotObserved = "not observed"
	LinkUnknown     = "unknown"

	// The three below are STRUCTURAL: they are facts about the host or about
	// an instruction the user gave, not observations of the wire. So they are
	// decided ABOVE the window gate and survive WindowApplied being false --
	// there is nothing for a window to bound.

	// LinkForgotten: `forget --host` suppressed this host. Rendered rather
	// than dropped. A link that silently omitted the host would make the
	// forget read as undone on the next report, and a silent omission is the
	// failure this whole program is arranged against -- the destinations
	// section renders a Suppressed count for exactly this reason.
	LinkForgotten = "forgotten"
	// LinkLoopback: loopback is never proxied, so no wire row can exist and
	// "not observed" would be a finding against every session that ever ran a
	// local server.
	LinkLoopback = "loopback"
	// LinkClientPlane: a host the client contacts on its own behalf every
	// session, so a row there is not evidence about this call.
	LinkClientPlane = "client plane"
)

// How one link ended. store.ExecOK, store.ExecFailed and store.ExecInterrupted
// are the recorded outcomes; these three are the cases where there is no
// outcome to report, kept apart because each is a different fact.
const (
	// LinkOutcomeDenied: the user refused the call at the permission prompt.
	// The product working, not the call failing.
	LinkOutcomeDenied = "denied by user"
	// LinkOutcomeUnobserved: an execution record exists with no outcome -- a v1
	// record read back. The call ran; how it ended was never written down.
	LinkOutcomeUnobserved = "outcome unobserved"
	// LinkOutcomeNoRecord: no execution record at all. The store's own comment
	// is the reason this is its own value: such a declaration was denied,
	// failed, or had its execution go unrecorded, and nothing in the record
	// knows which. Naming it "failed" would pick one and assert it.
	LinkOutcomeNoRecord = "no execution record"
)

// LinkHost is one hostname a call named, with what the wire says about it.
type LinkHost struct {
	Host  string `json:"host"`
	State string `json:"state"`
}

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
	// Hosts is what this call NAMED, never what it reached. The proxy's store
	// carries no tool_use_id, so no wire row can be attributed to an individual
	// call; a host's state here is the state of that host across this session's
	// window. The renderer says so in its legend, because a per-call reading is
	// the obvious misreading and it is the kind that overstates what is known.
	Hosts []LinkHost `json:"hosts"`
	// SSHHosts is kept APART from Hosts and carries no state, because there is
	// no observation to carry: the proxy cannot see ssh at all. Joining them
	// into one list puts a host the wire could never have shown beside hosts it
	// could, under a column that reads as a verdict on both.
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
// It reads `dests` rather than the raw observation on purpose: that view has
// already had forgotten hosts suppressed and client-plane traffic accounted
// for, and a second consumer reading around it is how a suppressed host comes
// back in a different section.
func buildChains(run *store.Run, dests Destinations, denied map[string]bool, forgotten func(string) bool) Chains {
	out := Chains{Prompts: []Chain{}, Unattributed: []Link{}, Dropped: []Link{}}
	if run == nil {
		return out
	}

	state := hostStates(dests)
	executed := executionsByID(run)

	type key struct{ transcript, prompt string }
	byKey := map[key]*Chain{}
	first := map[key]int64{}
	var order []key

	for _, d := range run.Declarations {
		if d.PromptID == nil || *d.PromptID == "" {
			out.Unattributed = append(out.Unattributed,
				buildLink(d, executed, denied, state, dests, forgotten))
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
		c.Links = append(c.Links, buildLink(d, executed, denied, state, dests, forgotten))
	}

	// Terminals whose declaration never landed. The id is all there is, and
	// saying so is the point: these are calls the run made and cannot describe.
	for _, id := range run.Dropped() {
		out.Dropped = append(out.Dropped, Link{
			ToolUseID: id,
			ToolName:  LinkUnknown,
			VerbClass: LinkUnknown,
			Outcome:   LinkUnknown,
			Outcomes:  []string{},
			Hosts:     []LinkHost{},
			SSHHosts:  []string{},
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
	out := map[string][]store.Execution{}
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

func buildLink(
	d store.Declaration,
	executed map[string][]store.Execution,
	denied map[string]bool,
	state map[string]string,
	dests Destinations,
	forgotten func(string) bool,
) Link {
	l := Link{
		Seq:       d.Seq,
		ToolUseID: d.ToolUseID,
		ToolName:  d.ToolName,
		VerbClass: d.Shape.VerbClass,
		Hosts:     []LinkHost{},
		SSHHosts:  []string{},
	}
	if d.Shape.Program != nil {
		l.Program = *d.Shape.Program
	}
	l.Outcome, l.Outcomes, l.ExecutionRecords = linkOutcome(d.ToolUseID, executed, denied)

	for _, h := range d.Hosts {
		l.Hosts = append(l.Hosts, LinkHost{Host: h, State: hostState(h, state, dests, forgotten)})
	}
	// Carried, never joined, and never given a state: the proxy cannot see ssh,
	// so there is no observation to report and a column that reads as a verdict
	// would be answering a question nobody could have asked of the wire.
	l.SSHHosts = append(l.SSHHosts, d.SSHHosts...)
	sort.Strings(l.SSHHosts)
	return l
}

// hostState is the per-link reading of a host.
//
// The window gate comes FIRST, above the lookup, and applies to the absent case
// as well as the present one. With no window enforced, "this host never
// appeared" is not a thing that was observed -- it is a thing that could not be
// observed -- and reporting it as the former is the failure this whole package
// is built to avoid.
func hostState(h string, state map[string]string, dests Destinations, forgotten func(string) bool) string {
	// THE THREE STRUCTURAL ANSWERS COME FIRST, above the window gate, because
	// none of them is an observation and a window bounds observations. Each one
	// replaced a `not observed` that was a false finding: forgotten made the
	// forget read as undone, loopback accused every session that ever ran a
	// local server, and client plane accused the client's own traffic.
	if forgotten != nil && forgotten(h) {
		return LinkForgotten
	}
	if loopbackHosts[h] {
		return LinkLoopback
	}
	// Read from the view's OWN client-plane list rather than from
	// clientPlaneHosts directly. The two differ in one case that matters:
	// mcp-proxy.anthropic.com belongs to the agent on a session that made
	// mcp__* calls, and the view already knows that. Re-deriving the predicate
	// here would put the chain and the destinations section into disagreement
	// about the same host in the same report.
	for _, c := range dests.ClientPlane {
		if c == h {
			return LinkClientPlane
		}
	}

	// The window gate applies to the ABSENT case as well as the present one.
	// With no window enforced, "this host never appeared" is not a thing that
	// was observed -- it is a thing that could not be observed.
	if !dests.WindowApplied {
		return LinkUnknown
	}
	if s, ok := state[h]; ok {
		return s
	}
	return LinkNotObserved
}

// hostStates reduces the destination rows to one state per host.
func hostStates(dests Destinations) map[string]string {
	out := map[string]string{}
	for _, h := range dests.Hosts {
		out[h.Host] = destinationState(h)
	}
	return out
}

// destinationState reads one destination.
//
// Reached wins over failed. The link answers whether this session got to the
// host, and a session that connected once and was refused three times did get
// there; reporting that as "attempted" would be false in the direction that
// understates what the agent did, which is the direction this report must never
// be wrong in.
func destinationState(d wire.Destination) string {
	switch {
	case d.InWindowReached > 0:
		return LinkReached
	case d.InWindowFailed > 0:
		return LinkAttempted
	default:
		// A row exists but neither counter moved: every attempt on it was
		// inherited, so it is another session's destination and this one has
		// observed nothing about it.
		return LinkNotObserved
	}
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
		if o == "" {
			o = LinkOutcomeUnobserved
		}
		all = append(all, o)
	}
	// The LAST record after the seq sort: the highest position wins. Both are
	// listed above it either way, so the pick decides emphasis and not what the
	// reader is allowed to see.
	return all[len(all)-1], all, len(recs)
}
