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
	// LinkNotObservable: an ssh host. The proxy cannot see ssh at all, so the
	// absence of a row is a property of the transport rather than evidence
	// about the call -- and unlike every other state here it survives
	// WindowApplied being false, because it was never an observation.
	LinkNotObservable = "not observable"
	LinkUnknown       = "unknown"
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
	Outcome   string `json:"outcome"`
	// Hosts is what this call NAMED, never what it reached. The proxy's store
	// carries no tool_use_id, so no wire row can be attributed to an individual
	// call; a host's state here is the state of that host across this session's
	// window. The renderer says so in its legend, because a per-call reading is
	// the obvious misreading and it is the kind that overstates what is known.
	Hosts []LinkHost `json:"hosts"`
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
	// Unchained counts declarations carrying no prompt id, which is every v1
	// record. They cannot be placed, and a view that dropped them silently
	// would read as complete while omitting real work.
	Unchained int `json:"unchained_calls"`
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
	out := Chains{Prompts: []Chain{}}
	if run == nil {
		return out
	}

	state := hostStates(dests)
	executed := map[string]store.Execution{}
	for _, e := range run.Executions {
		executed[e.ToolUseID] = e
	}

	type key struct{ transcript, prompt string }
	byKey := map[key]*Chain{}
	first := map[key]int64{}
	var order []key

	for _, d := range run.Declarations {
		if d.PromptID == nil || *d.PromptID == "" {
			out.Unchained++
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
		c.Links = append(c.Links, buildLink(d, executed, denied, state, dests.WindowApplied, forgotten))
	}

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

func buildLink(
	d store.Declaration,
	executed map[string]store.Execution,
	denied map[string]bool,
	state map[string]string,
	windowApplied bool,
	forgotten func(string) bool,
) Link {
	l := Link{
		Seq:       d.Seq,
		ToolUseID: d.ToolUseID,
		ToolName:  d.ToolName,
		VerbClass: d.Shape.VerbClass,
		Outcome:   linkOutcome(d.ToolUseID, executed, denied),
		Hosts:     []LinkHost{},
	}
	if d.Shape.Program != nil {
		l.Program = *d.Shape.Program
	}

	for _, h := range d.Hosts {
		if forgotten != nil && forgotten(h) {
			continue
		}
		l.Hosts = append(l.Hosts, LinkHost{Host: h, State: hostState(h, state, windowApplied)})
	}
	for _, h := range d.SSHHosts {
		if forgotten != nil && forgotten(h) {
			continue
		}
		// Not routed through hostState: this one is true whatever the window
		// did, because it is a fact about the transport and not an observation.
		l.Hosts = append(l.Hosts, LinkHost{Host: h, State: LinkNotObservable})
	}
	return l
}

// hostState is the per-link reading of a host.
//
// The window gate comes FIRST, above the lookup, and applies to the absent case
// as well as the present one. With no window enforced, "this host never
// appeared" is not a thing that was observed -- it is a thing that could not be
// observed -- and reporting it as the former is the failure this whole package
// is built to avoid.
func hostState(h string, state map[string]string, windowApplied bool) string {
	if !windowApplied {
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

func linkOutcome(id string, executed map[string]store.Execution, denied map[string]bool) string {
	if e, ok := executed[id]; ok {
		if e.Outcome == "" {
			return LinkOutcomeUnobserved
		}
		return e.Outcome
	}
	// Checked only when no execution record exists. A denial produces no
	// execution record, so the two cannot both be true -- and were a future
	// change to make them so, the record would be the stronger evidence.
	if denied[id] {
		return LinkOutcomeDenied
	}
	return LinkOutcomeNoRecord
}
