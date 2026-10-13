package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
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

	c := buildChains(run, executionsByID(run), nil)

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

	c := buildChains(run, executionsByID(run), nil)

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

	c := buildChains(run, executionsByID(run), nil)

	if len(c.Prompts) != 2 || c.Prompts[0].PromptID != "p1" {
		t.Fatalf("chains = %+v, want p1 first: it starts earlier", c.Prompts)
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

	c := buildChains(run, executionsByID(run), denied)
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

	got := buildChains(run, executionsByID(run), map[string]bool{"d1": true}).Prompts[0].Links[0].Outcome
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

	c := buildChains(run, executionsByID(run), nil)
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

	l := buildChains(run, executionsByID(run), nil).Prompts[0].Links[0]
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

	if got := buildChains(run, executionsByID(run), nil).Prompts[0].Links[0].Outcome; got != store.ExecFailed {
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

	c := buildChains(run, executionsByID(run), nil)
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

// TestChains_SSHHostsAreCarriedApart. The spec keeps ssh_hosts a separate
// field, never merged into DeclaredHosts: an ssh destination is a different kind of
// destination from an HTTPS one, and the family view treats it so.
func TestChains_SSHHostsAreCarriedApart(t *testing.T) {
	d := chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "wire.example")
	d.SSHHosts = []string{"git.example"}
	run := &store.Run{Declarations: []store.Declaration{d}}

	l := buildChains(run, executionsByID(run), nil).Prompts[0].Links[0]
	if len(l.DeclaredHosts) != 1 || l.DeclaredHosts[0] != "wire.example" {
		t.Errorf("declared_hosts = %+v, want only the one named outside ssh_hosts", l.DeclaredHosts)
	}
	if len(l.SSHHosts) != 1 || l.SSHHosts[0] != "git.example" {
		t.Errorf("ssh_hosts = %v, want the ssh host carried in its own field", l.SSHHosts)
	}
}

// TestChains_HostsRenderAsSortedNames pins what a link's hosts are now: the
// names the call declared, sorted, with nothing beside them. The retired
// per-host states left no test on a link that names more than one host.
func TestChains_HostsRenderAsSortedNames(t *testing.T) {
	d := chainDecl(1, "t1", "Bash", "p1", "/main.jsonl", "zeta.example", "alpha.example")
	run := &store.Run{Declarations: []store.Declaration{d}}

	l := buildChains(run, executionsByID(run), nil).Prompts[0].Links[0]
	if len(l.DeclaredHosts) != 2 || l.DeclaredHosts[0] != "alpha.example" || l.DeclaredHosts[1] != "zeta.example" {
		t.Errorf("declared_hosts = %v, want the two names sorted", l.DeclaredHosts)
	}
	if got := linkHosts(l.DeclaredHosts); got != "  -> alpha.example, zeta.example" {
		t.Errorf("linkHosts = %q, want the names and nothing beside them", got)
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
// copy shares its slices, so assigning into Links[j].DeclaredHosts[k] through a
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
				ToolUseID:     "t1",
				DeclaredHosts: []string{"internal.acme.example"},
			}},
		}}},
	}}}

	red := Redact(rep, []byte("k"))

	orig := rep.Sessions[0].Chains.Prompts[0]
	if orig.Links[0].DeclaredHosts[0] != "internal.acme.example" {
		t.Errorf("the ORIGINAL report's host was overwritten with %q; Redact wrote through "+
			"a shared backing array", orig.Links[0].DeclaredHosts[0])
	}
	if orig.TranscriptPath != "/Users/someone/projects/acme-corp/x.jsonl" {
		t.Errorf("the original transcript path was overwritten with %q", orig.TranscriptPath)
	}

	got := red.Sessions[0].Chains.Prompts[0]
	if got.Links[0].DeclaredHosts[0] == "internal.acme.example" {
		t.Error("the redacted copy still carries the hostname")
	}
	if got.TranscriptPath == orig.TranscriptPath {
		t.Error("the transcript path was not digested; it carries the project directory, " +
			"which is often the customer's name")
	}
}

// TestRedactChains_UsesTheSameKeyedDigestAsTheRestOfTheReport. One path, not
// two: a second digest that drifted from redactHost would make the same host
// unmatchable between the chain view and the destinations section of one
// report. Deleted and restored alongside the test above.
func TestRedactChains_UsesTheSameKeyedDigestAsTheRestOfTheReport(t *testing.T) {
	key := []byte("install-key")
	rep := &Report{Sessions: []Session{{
		Destinations: Destinations{DeclaredHosts: []string{"pypi.org"}},
		Chains: Chains{Prompts: []Chain{{
			PromptID: "p1",
			Links:    []Link{{DeclaredHosts: []string{"pypi.org"}}},
		}}},
	}}}

	red := Redact(rep, key)
	inDest := red.Sessions[0].Destinations.DeclaredHosts[0]
	inChain := red.Sessions[0].Chains.Prompts[0].Links[0].DeclaredHosts[0]

	if inDest != inChain {
		t.Errorf("one host digested two ways: %q in destinations, %q in the chain. A reader "+
			"cannot join them, which is the one thing a digest has to preserve.",
			inDest, inChain)
	}
}
