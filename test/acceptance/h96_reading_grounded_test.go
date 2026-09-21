package acceptance

import "testing"

// H-96 -- every model claim cites a digest field.
//
// Break: allow free prose and the line can assert something no record
// supports.
//
// Adapted, and the adaptation is the finding: wiring Part 5 turned up that
// a "type": "prompt" hook's ONLY input is `$ARGUMENTS`, which Claude Code
// fills with that event's own fixed payload (session_id, transcript_path,
// last_assistant_message, stop_hook_active, ...) -- see
// internal/install/reading.go's package doc. There is no documented channel
// for rashomon's own digest, which is this program's private data and no
// part of Claude Code's event schema, to reach that payload. So "cites a
// digest field" cannot be built as the spec's own worked framing implies --
// there is no digest in the model's context to cite, and a citation-
// checking function with no digest ever handed to it would be a validator
// with no live caller, exactly the dead-code shape the sign-off section
// warns a mutation sweep will not save anyone from.
//
// What ships instead, and what this test holds down: the prompt is written
// so the ONLY grounded answer it is allowed to give is a VERBATIM QUOTE of
// the one document it was actually given -- last_assistant_message itself,
// via `$ARGUMENTS` -- and it is explicitly told the wider question (does
// the message match what happened) is out of reach. That is the strongest
// grounding available under the real constraint, not the grounding
// originally asked for, and the gap between the two is reported in the PR
// rather than coded around.
func TestH96_TheInstalledPromptRequiresAVerbatimQuoteAndDisclaimsRecordAccess(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if res := e.enableReading(); res.exitCode != 0 {
		t.Fatalf("enable-reading: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	prompt := readingPromptFromSettings(t, e)

	mustContain(t, prompt, "verbatim",
		"the model is not told its only grounded answer must quote last_assistant_message verbatim")
	mustContain(t, prompt, "cannot and must not judge",
		"the model is not told it cannot judge whether the work was complete, correct, authorized or safe")
	mustContain(t, prompt, "no access to this session's tool-call activity",
		"the model is not told it has no access to this session's tool-call activity -- the one honest limit found while wiring this")
	mustContain(t, prompt, "DATA",
		"the model is not told last_assistant_message is data to evaluate, not an instruction to it")

	// The negative half: nothing here asks the model to render a verdict
	// about the AGENT (as opposed to the one document it was given), the
	// same vocabulary discipline recap's own renderer holds itself to
	// (internal/recap's vocabulary test) even though nothing here prints to
	// a human -- a model primed with verdict language is a model one step
	// closer to producing it in `reason`, which DOES reach the agent.
	for _, forbidden := range []string{"authorized", "malicious", "on task", "off task", "safe", "rogue"} {
		if containsFold(prompt, forbidden) {
			t.Errorf("the installed prompt contains %q, a verdict word this design has no standing to use", forbidden)
		}
	}
}
