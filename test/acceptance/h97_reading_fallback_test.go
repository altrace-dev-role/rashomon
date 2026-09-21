package acceptance

import "testing"

// H-97 -- a timeout falls back to Part 4's line exactly once.
//
// Break: allow a late response and the same turn is reported twice.
//
// The mechanism found while wiring Part 5 makes this a STRUCTURAL
// guarantee rather than a race to defend against: Part 4's `recap` command
// and Part 5's "type": "prompt" entry are two separate, parallel hook
// entries under the same event -- Anthropic's own hooks reference is
// explicit that "all matching hooks run in parallel" against an identical,
// unmodified copy of the input, and one hook's timeout, absence, or slow
// response cannot delay or duplicate what a SIBLING entry does, because
// there is no shared state between them at all. Part 4's own idempotency
// key (H-101, tested elsewhere) already bounds it to at most one line per
// (session_id, prompt_id) regardless.
//
// What THIS test can check, honestly, is the half that lives in this
// binary: that installing, enabling, or leaving the reading entry present
// changes NOTHING about how quickly or how often Part 4's own recap
// completes. Claude Code's own handling of a slow or timed-out "type":
// "prompt" hook is outside this binary's process boundary and outside what
// a `go test` here can observe -- see internal/install/reading.go's
// package doc for the fuller account.
func TestH97_RecapCompletesExactlyOnceRegardlessOfReadingState(t *testing.T) {
	for _, enable := range []bool{false, true} {
		e := newEnv(t)
		e.watched(testSession)
		if enable {
			if res := e.enableReading(); res.exitCode != 0 {
				t.Fatalf("enable-reading: exit %d, stderr %q", res.exitCode, res.stderr)
			}
		}
		e.mustHook(defaultPayload().build(t))
		e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))

		payload := stopPayload(testSession, "Done. Everything succeeded.", false)

		first, ok := e.recapLine(payload)
		if !ok {
			t.Fatalf("reading enabled=%v: expected a line on the first Stop firing", enable)
		}

		// A Stop hook that continues the conversation makes Stop fire again
		// for the SAME prompt (H-101's own scenario). Simulated here the
		// same way h101's test does: invoke recap a second time with the
		// same session and prompt.
		second, ok := e.recapLine(payload)
		if ok {
			t.Errorf("reading enabled=%v: a second Stop firing for the same turn printed again: %q "+
				"(first was %q)", enable, second, first)
		}
	}
}
