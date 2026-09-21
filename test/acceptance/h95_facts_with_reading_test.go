package acceptance

import "testing"

// H-95 -- the facts render with the reading, never instead of it.
//
// Break: render the model's prose alone and the only checkable half is
// gone.
//
// What this means MECHANICALLY, given what wiring Part 5 turned up: a
// "type": "prompt" hook's result never reaches this binary at all (Claude
// Code consumes {ok, reason, impossible} itself -- see
// internal/install/reading.go's package doc) -- so there is no code path in
// which Part 5 EVER has the opportunity to suppress Part 4's line, because
// Part 4's `recap` command does not know Part 5 exists. H-95 is therefore
// not a behaviour to implement; it is a STRUCTURAL guarantee to hold down
// so nobody accidentally coupled the two later. Every assertion below is the
// same check from a different angle: cmdRecap's output is identical whether
// the reading entry is absent, present, or was just toggled off again.
func TestH95_RecapIsIdenticalWithReadingAbsent(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	line, ok := e.recapLine(stopPayload(testSession, "Ran git status as requested.", false))
	if ok {
		t.Fatalf("expected a clean turn to print nothing, got %q", line)
	}
}

func TestH95_RecapFindingIsIdenticalWhetherOrNotReadingIsEnabled(t *testing.T) {
	baseline := runFailureTurnAndCaptureLine(t, false)
	withReading := runFailureTurnAndCaptureLine(t, true)

	if baseline != withReading {
		t.Errorf("Part 4's line changed depending on Part 5's on/off state:\n"+
			"reading disabled: %q\nreading enabled:  %q", baseline, withReading)
	}
}

// runFailureTurnAndCaptureLine builds one recorded failure (a PostToolUse
// whose tool_response is an error) and returns the exact line Part 4's
// recap prints for it, with the reading entry either left absent or
// installed first.
func runFailureTurnAndCaptureLine(t *testing.T, enableReading bool) string {
	t.Helper()
	e := newEnv(t)
	e.watched(testSession)
	if enableReading {
		if res := e.enableReading(); res.exitCode != 0 {
			t.Fatalf("enable-reading: exit %d, stderr %q", res.exitCode, res.stderr)
		}
	}

	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))

	line, ok := e.recapLine(stopPayload(testSession, "Done. Everything succeeded.", false))
	if !ok {
		t.Fatalf("expected a line for a session whose final message did not mention the failure")
	}
	return line
}
