package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
	// The legend explains a host's STATE, and a state comes only from a proxy
	// store. Without one there is no state on the page, so no legend either:
	// this alpha's reader would be told how to read a column they cannot see.

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
	if got := rep.Chains.Prompts[0].Links[0].Outcome; got != "denied before running" {
		t.Errorf("outcome = %q, want \"denied before running\". Without the transcript this is "+
			"indistinguishable from an unrecorded execution, and calling it one reports the "+
			"product working as the product broken.", got)
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
	if len(l.DeclaredHosts) != 0 {
		t.Errorf("declared_hosts = %+v, want none: the only host named is an ssh one", l.DeclaredHosts)
	}
	if len(l.SSHHosts) != 1 || l.SSHHosts[0] != "git.example.com" {
		t.Errorf("ssh_hosts = %v, want it carried in its own field", l.SSHHosts)
	}

	// Without a proxy store nothing is observable, so "(not observable)" would
	// single out the one host that is no different from the rest: the ssh host
	// is named, apart, and nothing more.
	out := e.run("", nil, "report", "--session", testSession, "--chain").stdout
	if !strings.Contains(out, "ssh: git.example.com\n") {
		t.Errorf("the render does not carry the ssh host apart, and plainly:\n%s", out)
	}

}

// TestH31_NonoTrailReachesTheCLI drives the flag end to end.
//
// Written because the adapter reached the report and NOT the command line: the
// seam test guarded Build -> buildNono while the wire that was actually cut
// was main -> WithNonoTrail, so every report a user could produce printed
// nothing about the sandbox. A guard one level below the break is not a guard.
func TestH31_NonoTrailReachesTheCLI(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.probe("end", testSession)

	// The real capture, copied where the binary can read it.
	raw, err := os.ReadFile(filepath.Join(moduleRoot, "test/fixtures/nono/audit-events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	trail := filepath.Join(e.home, "audit-events.ndjson")
	if err := os.WriteFile(trail, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	out := e.run("", nil, "report", "--session", testSession, "--nono-audit", trail).stdout
	if !strings.Contains(out, "sandbox (nono):") {
		t.Errorf("--nono-audit produced no sandbox section:\n%s", out)
	}
	// The events are from a real capture whose instants precede this test's
	// window, so they are inherited -- which the section must SAY rather than
	// rendering as a quiet sandbox.
	if !strings.Contains(out, "outside this session's window") {
		t.Errorf("the section does not account for the out-of-window events:\n%s", out)
	}

	// And without the flag, silence.
	plain := e.run("", nil, "report", "--session", testSession).stdout
	if strings.Contains(plain, "sandbox (nono)") {
		t.Errorf("a session with no trail configured rendered a sandbox line:\n%s", plain)
	}
}

// TestH31_ASilentTrailLeavesTheNamedHostInTheChain pins what the README's
// "Inside a sandbox" says about a trail with nothing in it. nono writes no
// event for a plain-HTTP request it allows, and none at all when it does not
// filter the network (measured with 0.78.0 and the claude profile), so the
// sandbox section says the trail holds no network events rather than
// counting zero -- a #53 follow-up: rendering `0 allowed, 0 denied` here
// read as a filtered session that saw nothing, when no filtering ran. The
// README said an allowed http:// request "appears nowhere in the report";
// the call that named the URL still does, and --chain lists the host under
// it. The trail here is that capture's shape: the session's start and end
// and no network event.
func TestH31_ASilentTrailLeavesTheNamedHostInTheChain(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl -sS -o /dev/null http://example.com/"}
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	now := time.Now()
	trail := filepath.Join(e.home, "audit-events.ndjson")
	body := fmt.Sprintf(`{"sequence":0,"event":{"type":"session_started","started":%q}}`+"\n"+
		`{"sequence":1,"event":{"type":"session_ended","ended":%q,"exit_code":0}}`+"\n",
		now.Add(-time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err := os.WriteFile(trail, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	out := e.run("", nil, "report", "--session", testSession, "--nono-audit", trail, "--chain").stdout
	if !strings.Contains(out, "sandbox (nono): the trail holds no network events") {
		t.Errorf("premise: a trail with no network event at all should say so, not count zero:\n%s", out)
	}
	if strings.Contains(out, "0 allowed, 0 denied") {
		t.Errorf("a trail that never wrote a network record rendered as a filtered, quiet session:\n%s", out)
	}
	if !regexp.MustCompile(`(?m)^\s+1\s+Bash .*-> example\.com$`).MatchString(out) {
		t.Errorf("the call that named http://example.com/ does not list the host in its chain:\n%s", out)
	}

	// The README says what this render shows, and nothing it contradicts.
	readme, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(readme)), " ")
	for _, claim := range readmeSilentTrailClaims {
		if !strings.Contains(flat, claim) {
			t.Errorf("README.md \"Inside a sandbox\" no longer says %q, which this render backs", claim)
		}
	}
	if strings.Contains(flat, "appears nowhere in the report") {
		t.Errorf("README.md says an allowed http:// request \"appears nowhere in the report\"; "+
			"--chain lists the host the call named:\n%s", out)
	}
}

// readmeSilentTrailClaims are the README's sentences that the render in
// TestH31_ASilentTrailLeavesTheNamedHostInTheChain backs, whitespace-folded.
//
// The second claim names the v1.1.0 rendering ("0 allowed, 0 denied") and
// says so, as the README does; the third names what a build from main
// prints (see writeNono in internal/report/text.go), which the test above
// asserts. The README scopes each reading to its version, so a claim that
// drops the scope would pin a sentence true only of v1.1.0. When a release
// ships the new wording, the README and these claims change together.
var readmeSilentTrailClaims = []string{
	"The sandbox section has something to count only when nono filters the network.",
	"In v1.1.0, with nono's `claude` profile, which leaves the network " +
		"unfiltered, the trail holds no network event at all, and the section " +
		"reads `0 allowed, 0 denied` however many hosts the session reached.",
	"A build from `main` says `the trail holds no network events` instead.",
	"An allowed `http://` request leaves nothing in the sandbox section.",
	"`--chain` still lists its host under the call if the command named it",
}
