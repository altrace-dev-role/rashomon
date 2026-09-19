package acceptance

import (
	"strings"
	"testing"
)

// H-31 -- the chain view, end to end.
//
// Every other section of the report is a SET: declarations, executions, hosts,
// findings. Sets answer "what happened in this session" and structurally cannot
// answer the question a user asks first -- "which of my requests caused that"
// -- because the edge between a prompt and the calls it produced is exactly
// what a set discards.
//
// Nothing here is new information. prompt_id and seq have been in every
// declaration since v2. These tests drive the real binary because the unit
// tests cannot show the thing that actually matters: that the spine survives
// the hook, the store, the reader and the renderer with its ordering intact.

// TestH31_ChainsGroupCallsUnderTheirPrompt is the item.
func TestH31_ChainsGroupCallsUnderTheirPrompt(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// Two prompts, three calls. The second prompt's call is recorded BETWEEN
	// the first prompt's two, which is the case a naive grouping gets right by
	// accident when the input happens to be sorted.
	for _, c := range []struct{ id, prompt, tool string }{
		{"toolu_a1", "prompt-1", "Bash"},
		{"toolu_b1", "prompt-2", "Read"},
		{"toolu_a2", "prompt-1", "Edit"},
	} {
		p := defaultPayload()
		p.ToolUseID, p.PromptID, p.ToolName = c.id, c.prompt, c.tool
		p.ToolInput = map[string]any{"command": "true"}
		e.mustHook(p.build(t))

		post := defaultPost()
		post.ToolUseID, post.ToolName, post.ToolInput = c.id, c.tool, p.ToolInput
		e.mustPost(post.build(t))
	}
	e.probe("end", testSession)

	rep := e.report(testSession)
	// THE PREMISE, ASSERTED. The first version of this test fired the post
	// payloads down the PreToolUse path, so each one was recorded as a second
	// DECLARATION: six declarations, zero executions, and three calls with no
	// prompt id landing in the unchained count. Every assertion below still
	// passed, because the three real declarations grouped correctly -- a test
	// green on a fixture that never exercised the outcome path at all.
	if rep.Executions.Recorded != 3 {
		t.Fatalf("executions recorded = %d, want 3. The fixture is not exercising the "+
			"execution path and the outcomes below mean nothing.", rep.Executions.Recorded)
	}
	if len(rep.Chains.Unattributed) != 0 {
		t.Fatalf("unattributed = %+v, want none: every declaration here carries a prompt id",
			rep.Chains.Unattributed)
	}
	if len(rep.Chains.Prompts) != 2 {
		t.Fatalf("chains = %d, want 2: two prompts produced these calls. %+v",
			len(rep.Chains.Prompts), rep.Chains.Prompts)
	}

	byPrompt := map[string][]string{}
	for _, c := range rep.Chains.Prompts {
		for _, l := range c.Links {
			byPrompt[c.PromptID] = append(byPrompt[c.PromptID], l.ToolUseID)
		}
	}
	if got := byPrompt["prompt-1"]; len(got) != 2 || got[0] != "toolu_a1" || got[1] != "toolu_a2" {
		t.Errorf("prompt-1 = %v, want [toolu_a1 toolu_a2] in that order. seq is the total "+
			"order and the chain is the one place the report claims a sequence.", got)
	}
	if got := byPrompt["prompt-2"]; len(got) != 1 || got[0] != "toolu_b1" {
		t.Errorf("prompt-2 = %v, want [toolu_b1]", got)
	}
	for _, c := range rep.Chains.Prompts {
		for _, l := range c.Links {
			if l.Outcome != "ok" {
				t.Errorf("%s outcome = %q, want ok: every call here has an execution record "+
					"saying it succeeded", l.ToolUseID, l.Outcome)
			}
		}
	}

	out := e.run("", nil, "report", "--session", testSession, "--chain").stdout
	if !strings.Contains(out, "chains: 2") {
		t.Errorf("the render does not carry the chain section:\n%s", out)
	}
	if !strings.Contains(out, "prompt prompt-1") {
		t.Errorf("the render does not name the prompt:\n%s", out)
	}
	// The legend is load-bearing, not decoration: a host beside a call reads as
	// though that call reached it, and the store cannot support that claim.
	if !strings.Contains(out, "not proof this call reached it") {
		t.Errorf("the render omits the legend that bounds what a link host means:\n%s", out)
	}
}

// TestH31_ADeniedCallRendersAsDeniedInItsChain joins the chain view to H-30.
//
// A denied call has a declaration and no execution, which is indistinguishable
// in the store from a call whose execution went unrecorded. Only the
// transcript's tool_result says which, and the chain has to reach it -- or it
// renders the user exercising the permission prompt as a recording failure, one
// layer further out than where H-30 fixed it.
func TestH31_ADeniedCallRendersAsDeniedInItsChain(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.TranscriptPath = writeResultTranscript(t, e, p.ToolUseID, deniedText, true)
	e.mustHook(p.build(t))
	// No post hook: a denied call never runs.
	e.probe("end", testSession)

	rep := e.report(testSession)
	if len(rep.Chains.Prompts) != 1 || len(rep.Chains.Prompts[0].Links) != 1 {
		t.Fatalf("want one chain with one link, got %+v", rep.Chains.Prompts)
	}
	if got := rep.Chains.Prompts[0].Links[0].Outcome; got != "denied by user" {
		t.Errorf("outcome = %q, want \"denied by user\". Without the transcript this is "+
			"indistinguishable from an unrecorded execution, and calling it one reports the "+
			"product working as the product broken.", got)
	}
}

// TestH31_NoProxyStoreMeansUnknownRatherThanNotObserved is the honesty
// property of the whole view, tested through the WIRING rather than the
// function.
//
// The unit test pins hostState given WindowApplied false. What it cannot show
// is that "no proxy store" actually reaches that flag -- and if it did not,
// every host a call named would render "not observed", which says the wire was
// watched and this host never appeared. With no store the wire was not watched
// at all. That is a recorder gap being rendered as a finding against the agent,
// in the most common configuration there is: a user who has not run the proxy.
func TestH31_NoProxyStoreMeansUnknownRatherThanNotObserved(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))
	post := defaultPost()
	post.ToolInput = p.ToolInput
	e.mustPost(post.build(t))
	e.probe("end", testSession)

	rep := e.report(testSession)
	if rep.Destinations.Observed {
		t.Fatal("premise: this env has no proxy store, so nothing should be observed")
	}
	hosts := rep.Chains.Prompts[0].Links[0].Hosts
	if len(hosts) != 1 || hosts[0].Host != "pypi.org" {
		t.Fatalf("hosts = %+v, want the one the command named", hosts)
	}
	if hosts[0].State != "unknown" {
		t.Errorf("state = %q, want \"unknown\". With no proxy store the wire was never "+
			"watched, and \"not observed\" asserts that it was and this host never appeared "+
			"-- a missing recorder rendered as a finding against the agent.", hosts[0].State)
	}
}

// TestH31_TheListingIsBehindAFlagAndTheCountIsNot.
//
// The chain section is the only one whose length grows with the session, so on
// a long day it buries a report whose other sections are fixed size. But hiding
// it entirely would be the opposite error -- a view nobody knows exists is the
// same as one that was never built -- so the count always renders and the flag
// only expands it.
func TestH31_TheListingIsBehindAFlagAndTheCountIsNot(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	e.mustHook(p.build(t))
	post := defaultPost()
	e.mustPost(post.build(t))
	e.probe("end", testSession)

	plain := e.run("", nil, "report", "--session", testSession).stdout
	if !strings.Contains(plain, "chains: 1") {
		t.Errorf("the default report does not say a chain exists:\n%s", plain)
	}
	if strings.Contains(plain, "prompt prompt-1") {
		t.Errorf("the default report expanded the listing:\n%s", plain)
	}

	expanded := e.run("", nil, "report", "--session", testSession, "--chain").stdout
	if !strings.Contains(expanded, "prompt prompt-1") {
		t.Errorf("--chain did not expand the listing:\n%s", expanded)
	}

	// JSON carries the structure either way: that reader is a program selecting
	// fields, and a consumer must not be able to parse a report and silently
	// miss a section because a flag was absent.
	if len(e.report(testSession).Chains.Prompts) != 1 {
		t.Error("the JSON form dropped the chain without --chain; a consumer selecting " +
			"fields would see a session with no chains rather than a flag it did not pass")
	}
}

// TestH31_TheChainSurvivesRedaction. --redact must digest the names inside the
// chain as well, with the SAME keyed digest the rest of the report uses -- a
// second digest would make one host unjoinable between two sections of one
// report.
func TestH31_TheChainSurvivesRedaction(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))

	post := defaultPost()
	post.ToolInput = p.ToolInput
	e.mustPost(post.build(t))
	e.probe("end", testSession)

	out := e.run("", nil, "report", "--session", testSession, "--redact").stdout
	if strings.Contains(out, "pypi.org") {
		t.Errorf("a redacted report still names the host inside the chain view:\n%s", out)
	}
	if !strings.Contains(out, "chains: 1") {
		t.Errorf("the chain section vanished under --redact; redaction removes names, not "+
			"structure:\n%s", out)
	}
}

// TestH31_LoopbackAndClientPlaneAreNotFindings drives the two cases the review
// predicted, end to end.
//
// Both were real in the first implementation of this view. A declared
// `curl http://localhost:3000` read "not observed" -- which says the wire was
// watched and this host never appeared -- on every session, forever, because
// loopback is never proxied and no row can ever exist. The same for the
// client's own traffic. Neither is a finding, and rendering them as one
// accuses the agent of the recorder's blind spots.
func TestH31_LoopbackAndClientPlaneAreNotFindings(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl http://localhost:3000/health"}
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	hosts := e.report(testSession).Chains.Prompts[0].Links[0].Hosts
	if len(hosts) != 1 {
		t.Fatalf("hosts = %+v, want the one named", hosts)
	}
	if hosts[0].State != "loopback" {
		t.Errorf("%s = %q, want \"loopback\". Loopback is never proxied, so no row can "+
			"exist and \"not observed\" accuses every session that ran a local server.",
			hosts[0].Host, hosts[0].State)
	}
}

// TestH31_SSHHostsAreCarriedApartEndToEnd. The proxy cannot see ssh, so an ssh
// host gets its own field and no state: a verdict column beside it would be
// answering a question the wire could never be asked.
func TestH31_SSHHostsAreCarriedApartEndToEnd(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "git clone ssh://git.example.com/infra.git"}
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	l := e.report(testSession).Chains.Prompts[0].Links[0]
	if len(l.Hosts) != 0 {
		t.Errorf("hosts = %+v, want none: the only host named is an ssh one", l.Hosts)
	}
	if len(l.SSHHosts) != 1 || l.SSHHosts[0] != "git.example.com" {
		t.Errorf("ssh_hosts = %v, want it carried in its own field", l.SSHHosts)
	}

	out := e.run("", nil, "report", "--session", testSession, "--chain").stdout
	if !strings.Contains(out, "ssh: git.example.com (not observable)") {
		t.Errorf("the render does not carry the ssh host apart:\n%s", out)
	}
}
