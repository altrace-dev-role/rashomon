package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// Part 1 -- the chain view: which prompt produced which calls, and what each
// call named.
//
// Everything else in the report is a SET. Declarations, executions, hosts,
// findings: flat collections answering "what happened in this session". The
// one question they cannot answer is the one a user actually asks first --
// "which of my requests caused that" -- because the causal edge between a
// prompt and the calls it produced is exactly what a set discards.
//
// The spine is already in the store: every declaration carries a prompt_id and
// a transcript_path, and seq is a total order. This assembles it. It invents
// nothing.

func chainDecl(seq int64, id, tool, promptID, transcript string, hosts ...string) store.Declaration {
	d := store.Declaration{
		Seq:            seq,
		ToolUseID:      id,
		ToolName:       tool,
		SessionID:      "s1",
		TranscriptPath: transcript,
		Hosts:          hosts,
		Shape:          shape.Shape{VerbClass: "network"},
	}
	if promptID != "" {
		p := promptID
		d.PromptID = &p
	}
	return d
}

// TestChains_KeyedByTranscriptAndPrompt is the reason the key is a pair.
//
// A prompt id is unique within a transcript and nowhere else. A session with a
// subagent has two transcripts, and keying on the prompt id alone silently
// welds two unrelated prompts into one chain -- a chain that asserts a causal
// edge that does not exist, which is worse than having no chain view at all.
func TestChains_KeyedByTranscriptAndPrompt(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl"),
		chainDecl(2, "t2", "Bash", "p1", "/subagent.jsonl"),
	}}

	c := buildChains(run, Destinations{}, nil, nil)

	if len(c.Prompts) != 2 {
		t.Fatalf("got %d chains, want 2. The same prompt id in two transcripts is two "+
			"prompts; merging them asserts a causal edge that does not exist: %+v",
			len(c.Prompts), c.Prompts)
	}
}

// TestChains_OrderedBySeq is the ordering fixture. Records arrive in whatever
// order the store yields them, and a chain whose links are out of order tells
// the reader the agent did things in an order it did not.
func TestChains_OrderedBySeq(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(3, "t3", "Edit", "p1", "/main.jsonl"),
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl"),
		chainDecl(2, "t2", "Read", "p1", "/main.jsonl"),
	}}

	c := buildChains(run, Destinations{}, nil, nil)

	if len(c.Prompts) != 1 {
		t.Fatalf("premise: one chain expected, got %d", len(c.Prompts))
	}
	var got []string
	for _, l := range c.Prompts[0].Links {
		got = append(got, l.ToolUseID)
	}
	want := []string{"t1", "t2", "t3"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("links = %v, want %v -- seq is the total order and the chain is the "+
				"one place in the report that claims a sequence", got, want)
		}
	}
}

// TestChains_ChainsOrderedByFirstSeq. Chains are ordered against each other the
// same way, by where they START, so the view reads down the session.
func TestChains_ChainsOrderedByFirstSeq(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(10, "t10", "Bash", "p2", "/main.jsonl"),
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl"),
		chainDecl(11, "t11", "Bash", "p2", "/main.jsonl"),
	}}

	c := buildChains(run, Destinations{}, nil, nil)

	if len(c.Prompts) != 2 || c.Prompts[0].PromptID != "p1" {
		t.Fatalf("chains = %+v, want p1 first: it starts earlier", c.Prompts)
	}
}

// TestChains_HostStates covers the three states a named host can be in when the
// window WAS applied.
func TestChains_HostStates(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "reached.example", "refused.example", "never.example"),
	}}
	d := Destinations{
		WindowApplied: true,
		Hosts: []wire.Destination{
			{Host: "reached.example", InWindowReached: 1},
			{Host: "refused.example", InWindowFailed: 2},
		},
	}

	c := buildChains(run, d, nil, nil)
	states := map[string]string{}
	for _, h := range c.Prompts[0].Links[0].Hosts {
		states[h.Host] = h.State
	}

	if states["reached.example"] != LinkReached {
		t.Errorf("reached.example = %q, want %q", states["reached.example"], LinkReached)
	}
	if states["refused.example"] != LinkAttempted {
		t.Errorf("refused.example = %q, want %q: the wire saw the attempt and no connection",
			states["refused.example"], LinkAttempted)
	}
	if states["never.example"] != LinkNotObserved {
		t.Errorf("never.example = %q, want %q: the call named it and no row exists",
			states["never.example"], LinkNotObserved)
	}
}

// TestChains_ReachedWinsOverFailed. A host with both a reached and a refused
// request in the window reads as reached: the question the link answers is
// whether this session got there, and once it did, it did.
func TestChains_ReachedWinsOverFailed(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "flaky.example"),
	}}
	d := Destinations{
		WindowApplied: true,
		Hosts:         []wire.Destination{{Host: "flaky.example", InWindowReached: 1, InWindowFailed: 3}},
	}

	if got := buildChains(run, d, nil, nil).Prompts[0].Links[0].Hosts[0].State; got != LinkReached {
		t.Errorf("state = %q, want %q", got, LinkReached)
	}
}

// TestChains_WindowNotAppliedMakesEveryHostUnknown.
//
// When the store's timestamps could not be parsed the window was not enforced,
// so no row can be attributed to this session at all. Every per-host value goes
// to unknown -- INCLUDING the ones that would otherwise read "not observed",
// which is the case worth pinning: "not observed" is a claim about the wire,
// and with no window there is no basis for any claim.
func TestChains_WindowNotAppliedMakesEveryHostUnknown(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "reached.example", "never.example"),
	}}
	d := Destinations{
		WindowApplied: false,
		Hosts:         []wire.Destination{{Host: "reached.example", InWindowReached: 1}},
	}

	for _, h := range buildChains(run, d, nil, nil).Prompts[0].Links[0].Hosts {
		if h.State != LinkUnknown {
			t.Errorf("%s = %q, want %q. Without a window nothing is attributable to this "+
				"session, and 'not observed' is as much a claim as 'reached'.",
				h.Host, h.State, LinkUnknown)
		}
	}
}

// TestChains_Outcomes. Four states, each meaning one thing, because the store's
// own comment says a declaration without an execution is one of three
// possibilities and nothing in the record knows which.
func TestChains_Outcomes(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			chainDecl(1, "ok", "Bash", "p1", "/main.jsonl"),
			chainDecl(2, "v1", "Bash", "p1", "/main.jsonl"),
			chainDecl(3, "denied", "Bash", "p1", "/main.jsonl"),
			chainDecl(4, "missing", "Bash", "p1", "/main.jsonl"),
		},
		Executions: []store.Execution{
			{ToolUseID: "ok", Outcome: store.ExecOK},
			{ToolUseID: "v1", Outcome: ""}, // a v1 record: it ran, the ending was not recorded
		},
	}
	denied := map[string]bool{"denied": true}

	c := buildChains(run, Destinations{}, denied, nil)
	got := map[string]string{}
	for _, l := range c.Prompts[0].Links {
		got[l.ToolUseID] = l.Outcome
	}

	for id, want := range map[string]string{
		"ok":      store.ExecOK,
		"v1":      LinkOutcomeUnobserved,
		"denied":  LinkOutcomeDenied,
		"missing": LinkOutcomeNoRecord,
	} {
		if got[id] != want {
			t.Errorf("%s outcome = %q, want %q", id, got[id], want)
		}
	}
}

// TestChains_DeniedIsNotAFailure guards the distinction 7f7cb06 established
// one layer down. A denial is the user refusing at the permission prompt, which
// is the product working, and it must not render as the agent's call failing.
func TestChains_DeniedIsNotAFailure(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "d1", "Bash", "p1", "/main.jsonl"),
	}}

	got := buildChains(run, Destinations{}, map[string]bool{"d1": true}, nil).Prompts[0].Links[0].Outcome
	if got == store.ExecFailed {
		t.Errorf("a denied call rendered as %q; the user refusing is not the call failing", got)
	}
	if got != LinkOutcomeDenied {
		t.Errorf("outcome = %q, want %q", got, LinkOutcomeDenied)
	}
}

// TestChains_UnchainedCallsAreCounted. A v1 declaration carries no prompt id
// and cannot be placed in any chain. Dropping such calls silently is the defect
// -- the chain view would look complete while omitting real work -- so they are
// counted and the count is rendered.
func TestChains_UnchainedCallsAreCounted(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl"),
		chainDecl(2, "t2", "Bash", "", "/main.jsonl"),
		chainDecl(3, "t3", "Bash", "", "/main.jsonl"),
	}}

	c := buildChains(run, Destinations{}, nil, nil)
	if c.Unchained != 2 {
		t.Errorf("unchained = %d, want 2. A call with no prompt id cannot be placed, and a "+
			"chain view that drops it silently reads as complete while omitting real work.",
			c.Unchained)
	}
	if len(c.Prompts) != 1 {
		t.Errorf("chains = %+v, want only the one with a prompt id", c.Prompts)
	}
}

// TestChains_ForgottenHostsAreDroppedFromLinks.
//
// `forget --host` suppresses a destination from the report's view because the
// proxy's store is not ours to delete from. The chain reads the DECLARATION
// side, which is a second place the name lives -- so without this the forgotten
// host reappears here, and the forget reads as though it had been undone.
func TestChains_ForgottenHostsAreDroppedFromLinks(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "keep.example", "forgotten.example"),
	}}
	forgotten := func(h string) bool { return h == "forgotten.example" }

	hosts := buildChains(run, Destinations{WindowApplied: true}, nil, forgotten).Prompts[0].Links[0].Hosts
	for _, h := range hosts {
		if h.Host == "forgotten.example" {
			t.Errorf("a forgotten host reappeared on the declaration side: %+v", hosts)
		}
	}
	if len(hosts) != 1 {
		t.Errorf("hosts = %+v, want only keep.example", hosts)
	}
}

// TestRedactChains_DoesNotWriteThroughToTheOriginal is the aliasing test, and
// it is the reason redactChains rebuilds all three slices rather than copying
// the struct.
//
// Redact returns a NEW report so the caller can still render the original --
// `report` and `report --redact` are one code path with a flag. But a Go struct
// copy shares its slices, so assigning into Links[j].Hosts[k].Host through a
// shallow copy writes the digest into the original's backing array. The
// unredacted render would print digests, and a second render of the same
// in-memory report would look correctly redacted while sharing state with an
// object the caller believes is untouched. For a function whose users are
// deciding what is safe to send someone, that is the worst direction to fail in.
func TestRedactChains_DoesNotWriteThroughToTheOriginal(t *testing.T) {
	rep := &Report{Sessions: []Session{{
		Chains: Chains{Prompts: []Chain{{
			TranscriptPath: "/Users/someone/projects/acme-corp/x.jsonl",
			PromptID:       "p1",
			Links: []Link{{
				ToolUseID: "t1",
				Hosts:     []LinkHost{{Host: "internal.acme.example", State: LinkReached}},
			}},
		}}},
	}}}

	red := Redact(rep, []byte("k"))

	orig := rep.Sessions[0].Chains.Prompts[0]
	if orig.Links[0].Hosts[0].Host != "internal.acme.example" {
		t.Errorf("the ORIGINAL report's host was overwritten with %q; Redact wrote through "+
			"a shared backing array", orig.Links[0].Hosts[0].Host)
	}
	if orig.TranscriptPath != "/Users/someone/projects/acme-corp/x.jsonl" {
		t.Errorf("the original transcript path was overwritten with %q", orig.TranscriptPath)
	}

	got := red.Sessions[0].Chains.Prompts[0]
	if got.Links[0].Hosts[0].Host == "internal.acme.example" {
		t.Error("the redacted copy still carries the hostname")
	}
	if got.TranscriptPath == orig.TranscriptPath {
		t.Error("the transcript path was not digested; it carries the project directory, " +
			"which is often the customer's name")
	}
	if got.Links[0].Hosts[0].State != LinkReached {
		t.Errorf("state = %q; the verdict is not a name and is the only thing left worth "+
			"reading", got.Links[0].Hosts[0].State)
	}
}

// TestRedactChains_UsesTheSameKeyedDigestAsTheRestOfTheReport. One path, not
// two: a second digest that drifted from redactHost would make the same host
// unmatchable between the chain view and the destinations section of one
// report.
func TestRedactChains_UsesTheSameKeyedDigestAsTheRestOfTheReport(t *testing.T) {
	key := []byte("install-key")
	rep := &Report{Sessions: []Session{{
		Destinations: Destinations{WireOnly: []string{"pypi.org"}},
		Chains: Chains{Prompts: []Chain{{
			PromptID: "p1",
			Links:    []Link{{Hosts: []LinkHost{{Host: "pypi.org", State: LinkReached}}}},
		}}},
	}}}

	red := Redact(rep, key)
	inDest := red.Sessions[0].Destinations.WireOnly[0]
	inChain := red.Sessions[0].Chains.Prompts[0].Links[0].Hosts[0].Host

	if inDest != inChain {
		t.Errorf("one host digested two ways: %q in destinations, %q in the chain. A reader "+
			"cannot join them, which is the one thing a digest has to preserve.",
			inDest, inChain)
	}
}

// TestChains_SSHHostsAreNotObservableRatherThanMissing. The proxy cannot see
// ssh at all, so an ssh host with no row is expected. Rendering it as "named
// and never seen" would accuse the session of hiding traffic the wire was never
// able to show.
func TestChains_SSHHostsAreNotObservableRatherThanMissing(t *testing.T) {
	d := chainDecl(1, "t1", "Bash", "p1", "/main.jsonl")
	d.SSHHosts = []string{"git.example"}
	run := &store.Run{Declarations: []store.Declaration{d}}

	hosts := buildChains(run, Destinations{WindowApplied: true}, nil, nil).Prompts[0].Links[0].Hosts
	if len(hosts) != 1 || hosts[0].Host != "git.example" {
		t.Fatalf("hosts = %+v, want the ssh host carried", hosts)
	}
	if hosts[0].State != LinkNotObservable {
		t.Errorf("state = %q, want %q: the proxy cannot see ssh, so its absence from the "+
			"wire is not evidence about the call", hosts[0].State, LinkNotObservable)
	}
}
