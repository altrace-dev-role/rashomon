package acceptance

import (
	"strings"
	"testing"
)

// rv3Bare is a peer message carrying body, as the hook receives it (Claude
// Code 2.1.322): the bare element, with no lead line and no closing
// paragraph; the attribute names a recorded message's, the values made up.
// internal/harness's frame test says what was measured.
func rv3Bare(body string) string {
	return `<cross-session-message from="uds:/tmp/cc-socks/1.sock" from-name="peer-a" from-mode="bypass">` + "\n" +
		body + "\n</cross-session-message>"
}

// deliveredDone counts the session's delivered done notes.
func (e *env) deliveredDone() int {
	n := 0
	for _, l := range e.doneLines() {
		if l.str("state") == "delivered" {
			n++
		}
	}
	return n
}

// TestRV1_3c_DoneNoteNeverNamesAFramedTurn: after a flagged Stop, a prompt
// the host injected (a peer's message, as the hook receives it) carries no
// done note; the framed turn runs and ends with its own Stop, which makes no
// claim; then the user types. The note never names a turn it is not about:
// at the typed prompt the previous turn is the framed one, so no note there
// says "the previous turn", and the session delivers the note at most once.
// Control: a peer message with text typed after it is a typed prompt, and
// the note is delivered there as at any typed prompt. Break: tell the note
// at a framed prompt, or skip the flagged Stop's take there, which holds the
// note past the framed turn's Stop to a typed prompt where it names the
// wrong turn.
func TestRV1_3c_DoneNoteNeverNamesAFramedTurn(t *testing.T) {
	t.Run("control_typed_after_frame", func(t *testing.T) {
		e := newSteerEnv(t)
		e.flaggedStop(t)
		res, out := e.prompt(e.cwd, rv3Bare("rebase onto main")+"\n\nand check the tests first", "prompt-2")
		if res.exitCode != 0 {
			t.Fatalf("prompt-2: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		// Contains, not equal: a task-preflight on the typed text may add its
		// own context beside the note.
		if !strings.Contains(out.HookSpecificOutput.AdditionalContext, doneNote) {
			t.Errorf("typed prompt-2 additionalContext = %q, want the done note in it", out.HookSpecificOutput.AdditionalContext)
		}
	})
	t.Run("framed_then_typed", func(t *testing.T) {
		e := newSteerEnv(t)
		e.flaggedStop(t)
		res, out := e.prompt(e.cwd, rv3Bare("rebase onto main and run the tests"), "prompt-2")
		if res.exitCode != 0 {
			t.Fatalf("prompt-2: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if strings.Contains(out.HookSpecificOutput.AdditionalContext, doneNote) || strings.Contains(out.SystemMessage, doneNote) {
			t.Errorf("framed prompt-2 carried the done note: context %q, system %q", out.HookSpecificOutput.AdditionalContext, out.SystemMessage)
		}
		for _, l := range e.doneLines() {
			if l.str("prompt_id") == "prompt-2" && l.str("state") == "delivered" {
				t.Errorf("a done note was delivered at framed prompt-2: %v", l)
			}
		}

		// The framed turn: one call of its own and its own Stop, no claim.
		e.turnCall(t, "prompt-2", "toolu_test_p2", goTestRun("ok"))
		e.recapLine(stopEvent("Stop", "Ran the tests for the other session.", e.cwd))

		out3 := noteOf(t, e, "prompt-3")
		ctx := out3.HookSpecificOutput.AdditionalContext
		t.Logf("typed prompt-3 additionalContext: %q", ctx)
		if strings.Contains(ctx, "the previous turn") {
			t.Errorf("typed prompt-3 says %q: the previous turn is the framed one, which made no claim", ctx)
		}
		if n := e.deliveredDone(); n > 1 {
			t.Errorf("%d done notes delivered, want at most 1", n)
		}
	})
}
