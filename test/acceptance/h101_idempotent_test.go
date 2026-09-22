package acceptance

import (
	"testing"
)

// H-101 -- one turn produces at most one line.
//
// A Stop hook that continues the conversation makes Stop fire AGAIN for the
// same prompt; stop_hook_active guards recursion, not duplicate output. This
// drives exactly that shape without needing a real continuation: the same
// (session, prompt) Stop payload, sent twice, the second time with
// stop_hook_active true (as Claude Code would set it on the replay). recap
// must speak once and stay silent the second time -- not because it looked
// at stop_hook_active, but because it remembers it already spoke for this
// turn (see internal/recap.Claim).
//
// Break: drop the idempotency key and the same finding prints twice, which
// for a tool whose value is that you can trust its counts is worse than not
// printing at all.
func TestH101_OneTurnProducesAtMostOneLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.PromptID = "prompt-1"
	e.mustHook(p.build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))

	line1, ok1 := e.recapLine(stopPayload(testSession, "Ran the command as requested.", false))
	if !ok1 {
		t.Fatal("the first Stop for this turn produced no line")
	}

	// The replay: same session, same turn (selectTurn resolves the "current"
	// turn to the same prompt id, since nothing new started), stop_hook_active
	// now true.
	line2, ok2 := e.recapLine(stopPayload(testSession, "Ran the command as requested.", true))
	if ok2 {
		t.Errorf("the second Stop for the SAME turn printed a second line: %q (first was %q)", line2, line1)
	}
}

// TestH101_ANewTurnAfterAClaimedOneStillSpeaks guards against the over-broad
// fix in the other direction: a session-wide "already spoke once" flag would
// silence every later turn, which is exactly what H-91 already rules out.
// This is the same property, checked from the idempotency mechanism's own
// angle rather than the turn-scoping angle.
func TestH101_ANewTurnAfterAClaimedOneStillSpeaks(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p1 := defaultPayload()
	p1.PromptID = "prompt-1"
	p1.ToolUseID = "toolu_1"
	e.mustHook(p1.build(t))
	e.mustPost(failurePayload(t, "toolu_1", "Exit code 1", false, 30))
	if _, ok := e.recapLine(stopPayload(testSession, "Ran it.", false)); !ok {
		t.Fatal("turn one produced no line")
	}

	p2 := defaultPayload()
	p2.PromptID = "prompt-2"
	p2.ToolUseID = "toolu_2"
	e.mustHook(p2.build(t))
	e.mustPost(failurePayload(t, "toolu_2", "Exit code 1", false, 30))
	if _, ok := e.recapLine(stopPayload(testSession, "Ran that one too.", false)); !ok {
		t.Error("turn two, a genuinely new prompt with its own failure, was silenced by turn " +
			"one's claim")
	}
}
