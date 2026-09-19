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

// TestChains_UnattributedCallsKeepTheirIDs.
//
// A v1 declaration carries no prompt id, and so does a subagent call whose
// payload never had one. They cannot be placed under a prompt.
//
// They are kept as LINKS rather than counted, and the difference is what makes
// H-31's set equality checkable at all: a count says how much is missing and a
// set equality needs to know WHICH, so only the second can prove nothing
// vanished. The first version of this carried a number.
func TestChains_UnattributedCallsKeepTheirIDs(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl"),
		chainDecl(2, "t2", "Bash", "", "/main.jsonl"),
		chainDecl(3, "t3", "Bash", "", "/main.jsonl"),
	}}

	c := buildChains(run, Destinations{}, nil, nil)
	if len(c.Unattributed) != 2 {
		t.Fatalf("unattributed = %+v, want the two calls with no prompt id", c.Unattributed)
	}
	if c.Unattributed[0].ToolUseID != "t2" || c.Unattributed[1].ToolUseID != "t3" {
		t.Errorf("unattributed = %+v, want t2 then t3 in seq order", c.Unattributed)
	}
	if len(c.Prompts) != 1 {
		t.Errorf("chains = %+v, want only the one with a prompt id", c.Prompts)
	}
}

// TestChains_ForgottenHostsReadAsForgotten replaces a test that asserted the
// opposite, and the correction is the point.
//
// I had the link DROP a forgotten host. That is more private and less honest,
// and honesty is the product: a link that silently omits a host makes the next
// report read as though the forget had been undone, and a silent omission is
// the exact failure every other line of this program is arranged against. The
// destinations section renders a Suppressed COUNT for precisely this reason
// rather than quietly shortening its list.
//
// The name is still gone from the rendered state -- `forgotten` says a host was
// suppressed here, not which one -- so nothing is leaked by saying so.
func TestChains_ForgottenHostsReadAsForgotten(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "keep.example", "forgotten.example"),
	}}
	forgotten := func(h string) bool { return h == "forgotten.example" }

	hosts := buildChains(run, Destinations{WindowApplied: true}, nil, forgotten).Prompts[0].Links[0].Hosts
	if len(hosts) != 2 {
		t.Fatalf("hosts = %+v, want both: a suppressed host is rendered as suppressed, "+
			"not omitted", hosts)
	}
	states := map[string]string{}
	for _, h := range hosts {
		states[h.Host] = h.State
	}
	if states["forgotten.example"] != LinkForgotten {
		t.Errorf("forgotten.example = %q, want %q. Dropping it makes the next report read "+
			"as though the forget had been undone.", states["forgotten.example"], LinkForgotten)
	}
	if states["keep.example"] != LinkNotObserved {
		t.Errorf("keep.example = %q; only the forgotten host changes", states["keep.example"])
	}
}

// TestChains_LoopbackIsNotAFinding is the case the review predicted verbatim:
// "a declared curl http://localhost:3000 reads not_observed_in_window on every
// session (loopback is never proxied)".
//
// It did. `not observed` is a CLAIM -- the wire was watched and this host never
// appeared -- and for loopback it is wrong on every session that ever ran a
// local server, forever, because loopback is never proxied and no row can
// exist.
func TestChains_LoopbackIsNotAFinding(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "localhost", "127.0.0.1"),
	}}

	for _, h := range buildChains(run, Destinations{WindowApplied: true}, nil, nil).Prompts[0].Links[0].Hosts {
		if h.State != LinkLoopback {
			t.Errorf("%s = %q, want %q: loopback is never proxied, so no row can exist and "+
				"'not observed' accuses every session that ran a local server",
				h.Host, h.State, LinkLoopback)
		}
	}
}

// TestChains_ClientPlaneReadsFromTheViewNotThePredicate.
//
// The state comes from the destinations view's OWN client-plane list rather
// than from clientPlaneHosts directly, and the difference is not cosmetic:
// mcp-proxy.anthropic.com belongs to the AGENT on a session that made mcp__*
// calls, and the view already knows that. Re-deriving the predicate here would
// put two sections of one report into disagreement about one host.
func TestChains_ClientPlaneReadsFromTheViewNotThePredicate(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "api.anthropic.com"),
	}}
	d := Destinations{WindowApplied: true, ClientPlane: []string{"api.anthropic.com"}}

	got := buildChains(run, d, nil, nil).Prompts[0].Links[0].Hosts[0].State
	if got != LinkClientPlane {
		t.Errorf("state = %q, want %q", got, LinkClientPlane)
	}

	// The same host, with the view NOT listing it as client plane -- which is
	// what an mcp-attributed session produces for the mcp proxy host. The link
	// must follow the view.
	d2 := Destinations{WindowApplied: true, ClientPlane: nil}
	if got := buildChains(run, d2, nil, nil).Prompts[0].Links[0].Hosts[0].State; got == LinkClientPlane {
		t.Error("the link called it client plane while the view did not. The view is the " +
			"one that knows about mcp attribution; two sections of one report must not " +
			"disagree about one host.")
	}
}

// TestChains_StructuralStatesSurviveNoWindow. forgotten, loopback and client
// plane are facts about a host or about an instruction the user gave -- not
// observations -- so a window has nothing to bound and they must not collapse
// to unknown with everything else.
func TestChains_StructuralStatesSurviveNoWindow(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		chainDecl(1, "t1", "Bash", "p1", "/main.jsonl",
			"localhost", "api.anthropic.com", "gone.example", "ordinary.example"),
	}}
	d := Destinations{WindowApplied: false, ClientPlane: []string{"api.anthropic.com"}}
	forgotten := func(h string) bool { return h == "gone.example" }

	want := map[string]string{
		"localhost":         LinkLoopback,
		"api.anthropic.com": LinkClientPlane,
		"gone.example":      LinkForgotten,
		"ordinary.example":  LinkUnknown,
	}
	for _, h := range buildChains(run, d, nil, forgotten).Prompts[0].Links[0].Hosts {
		if h.State != want[h.Host] {
			t.Errorf("%s = %q, want %q", h.Host, h.State, want[h.Host])
		}
	}
}

// TestChains_TwoPostRecordsKeepBoth. PostToolUse and PostToolUseFailure can
// both write for one id. The previous version kept a bare map assigned in slice
// order, so the last record read won silently -- and in the two-record case
// that is a coin toss that can discard the FAILURE, which is the half a reader
// most needs.
func TestChains_TwoPostRecordsKeepBoth(t *testing.T) {
	lo, hi := int64(1), int64(2)
	run := &store.Run{
		Declarations: []store.Declaration{chainDecl(1, "t1", "Bash", "p1", "/m.jsonl")},
		Executions: []store.Execution{
			// Deliberately reversed in the slice: the pick must come from seq.
			{ToolUseID: "t1", Outcome: store.ExecFailed, Seq: &hi},
			{ToolUseID: "t1", Outcome: store.ExecOK, Seq: &lo},
		},
	}

	l := buildChains(run, Destinations{}, nil, nil).Prompts[0].Links[0]
	if l.ExecutionRecords != 2 {
		t.Errorf("execution_records = %d, want 2", l.ExecutionRecords)
	}
	if l.Outcome != store.ExecFailed {
		t.Errorf("outcome = %q, want %q -- the HIGHER seq wins, not the later slice index",
			l.Outcome, store.ExecFailed)
	}
	if len(l.Outcomes) != 2 {
		t.Errorf("outcomes = %v, want both listed: showing only the winner hides the "+
			"disagreement worth seeing", l.Outcomes)
	}
}

// TestChains_ARecordWithNoSeqDoesNotOutrankOne. Execution.Seq is a nullable
// pointer: a record written to the spill file when the append lock could not be
// taken lands without a position. Letting an unknown position win would make
// the pick rule arbitrary again by another route.
func TestChains_ARecordWithNoSeqDoesNotOutrankOne(t *testing.T) {
	seq := int64(5)
	run := &store.Run{
		Declarations: []store.Declaration{chainDecl(1, "t1", "Bash", "p1", "/m.jsonl")},
		Executions: []store.Execution{
			{ToolUseID: "t1", Outcome: store.ExecFailed, Seq: &seq},
			{ToolUseID: "t1", Outcome: store.ExecOK, Seq: nil},
		},
	}

	if got := buildChains(run, Destinations{}, nil, nil).Prompts[0].Links[0].Outcome; got != store.ExecFailed {
		t.Errorf("outcome = %q, want %q: the record with a known position wins", got, store.ExecFailed)
	}
}

// TestChains_EveryDeclarationIsReachable is H-31's set equality. Nothing may
// vanish: a declaration lands under a prompt or in the unattributed group, and
// a terminal with no declaration gets a link carrying the one thing known
// about it.
func TestChains_EveryDeclarationIsReachable(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{
			chainDecl(1, "t1", "Bash", "p1", "/m.jsonl"),
			chainDecl(2, "t2", "Bash", "", "/m.jsonl"),
		},
		Terminals: []store.Terminal{{ToolUseID: "t_dropped"}},
	}

	c := buildChains(run, Destinations{}, nil, nil)
	seen := map[string]bool{}
	for _, ch := range c.Prompts {
		for _, l := range ch.Links {
			seen[l.ToolUseID] = true
		}
	}
	for _, l := range append(append([]Link{}, c.Unattributed...), c.Dropped...) {
		seen[l.ToolUseID] = true
	}

	for _, id := range []string{"t1", "t2", "t_dropped"} {
		if !seen[id] {
			t.Errorf("%s appears in no chain, no unattributed group and no dropped link. "+
				"A view that loses a call reads as a complete account of the session.", id)
		}
	}
	if len(c.Dropped) != 1 || c.Dropped[0].ToolName != LinkUnknown {
		t.Errorf("dropped = %+v; a terminal with no declaration is one link whose every "+
			"field but the id is unknown", c.Dropped)
	}
}

// TestChains_SSHHostsAreCarriedApart replaces a test that joined them into the
// host list with a `not observable` state. The spec keeps ssh_hosts a separate,
// never-joined field, and the reason is sound: a verdict column beside a host
// the wire could never have shown is answering a question nobody could ask.
func TestChains_SSHHostsAreCarriedApart(t *testing.T) {
	d := chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "wire.example")
	d.SSHHosts = []string{"git.example"}
	run := &store.Run{Declarations: []store.Declaration{d}}

	l := buildChains(run, Destinations{WindowApplied: true}, nil, nil).Prompts[0].Links[0]
	if len(l.Hosts) != 1 || l.Hosts[0].Host != "wire.example" {
		t.Errorf("hosts = %+v, want only the wire-observable one", l.Hosts)
	}
	if len(l.SSHHosts) != 1 || l.SSHHosts[0] != "git.example" {
		t.Errorf("ssh_hosts = %v, want the ssh host carried in its own field", l.SSHHosts)
	}
}

// TestRedactChains_SSHHostsAreDigestedToo closes a gap a mutation found: with
// ssh hosts moved to their own field, nothing asserted they were redacted at
// all.
//
// They are the hostnames most likely to be worth hiding. A public package index
// says little about an organisation; a bastion, a deploy target or an internal
// git host is the organisation's own topology, and it is exactly the name that
// would have survived in clear because it sat in a different field from the one
// the redaction test was watching.
func TestRedactChains_SSHHostsAreDigestedToo(t *testing.T) {
	rep := &Report{Sessions: []Session{{
		Chains: Chains{Prompts: []Chain{{
			PromptID: "p1",
			Links:    []Link{{ToolUseID: "t1", SSHHosts: []string{"bastion.internal.example"}}},
		}}},
	}}}

	red := Redact(rep, []byte("k"))
	got := red.Sessions[0].Chains.Prompts[0].Links[0].SSHHosts

	if len(got) != 1 {
		t.Fatalf("ssh_hosts = %v, want the one host carried through redaction", got)
	}
	if got[0] == "bastion.internal.example" {
		t.Error("a redacted report still names the ssh host in clear. It sits in its own " +
			"field, which is precisely why it was missed: the redaction test was watching " +
			"the other one.")
	}
	if rep.Sessions[0].Chains.Prompts[0].Links[0].SSHHosts[0] != "bastion.internal.example" {
		t.Error("the ORIGINAL report's ssh host was overwritten; the copy shares its slice")
	}
}

// TestRedactChains_DoesNotWriteThroughToTheOriginal is the aliasing test, and
// it is the reason redactLinks rebuilds every slice rather than copying the
// struct.
//
// Redact returns a NEW report so the caller can still render the original --
// `report` and `report --redact` are one code path with a flag. But a Go struct
// copy shares its slices, so assigning into Links[j].Hosts[k].Host through a
// shallow copy writes the digest into the original's backing array. The
// unredacted render would print digests, and a second render of the same
// in-memory report would look correctly redacted while sharing state with an
// object the caller believes is untouched. For a function whose users are
// deciding what is safe to send someone, that is the worst direction to fail in.
//
// THIS TEST WAS DELETED AND RESTORED. A text-range edit during the Part 1
// rework replaced a block that happened to contain it, the suite stayed green
// because a suite with fewer tests is still a green suite, and only the
// mutation sweep noticed -- the aliasing break stopped being detected while
// every gate still passed. That is the argument for the sweep in one line.
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
// report. Deleted and restored alongside the test above.
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
