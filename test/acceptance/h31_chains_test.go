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
	if rep.Chains.Unchained != 0 {
		t.Fatalf("unchained = %d, want 0: every declaration here carries a prompt id",
			rep.Chains.Unchained)
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

	out := e.run("", nil, "report", "--session", testSession).stdout
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
