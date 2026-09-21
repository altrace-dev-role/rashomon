package acceptance

import (
	"strings"
	"testing"
)

// H-91 -- a turn with a finding prints once, naming it.
//
// Break: render from session scope and a later clean turn repeats an
// earlier finding. This test exercises exactly that shape: turn one has a
// recorded failure the final message never mentions, and it must print,
// naming the count; turn two in the SAME session is clean, and must print
// nothing. A digest scoped to the session rather than the turn would have
// turn two repeat turn one's failure -- the digest never resets, and the
// line would fire on every turn from that point on.
func TestH91_AFindingPrintsOnceAndALaterCleanTurnStaysSilent(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// Turn one: a declared call that fails, and a summary naming none of the
	// failure vocabulary (account.go's list).
	p1 := defaultPayload()
	p1.PromptID = "prompt-1"
	p1.ToolUseID = "toolu_1"
	e.mustHook(p1.build(t))
	e.mustPost(failurePayload(t, "toolu_1", "Exit code 1", false, 30))

	line, ok := e.recapLine(stopPayload(testSession, "Ran the command as requested.", false))
	if !ok {
		t.Fatal("turn one has a recorded failure with a silent summary; recap printed nothing")
	}
	if !strings.HasPrefix(line, "※ rashomon: ") {
		t.Errorf("line = %q, does not open with the mark", line)
	}
	if !strings.Contains(line, "1 recorded failure") {
		t.Errorf("line = %q, want it to name the failure count", line)
	}
	if !strings.Contains(line, "--session "+testSession) {
		t.Errorf("line = %q, want the report command naming this session", line)
	}

	// Turn two, same session: a second, healthy call and an honest summary.
	// Nothing about turn one should reappear.
	p2 := defaultPayload()
	p2.PromptID = "prompt-2"
	p2.ToolUseID = "toolu_2"
	e.mustHook(p2.build(t))
	post2 := defaultPost()
	post2.ToolUseID = "toolu_2"
	e.mustPost(post2.build(t))

	line2, ok2 := e.recapLine(stopPayload(testSession, "Ran the second command too, both succeeded.", false))
	if ok2 {
		t.Errorf("turn two is clean but recap printed %q -- turn one's finding leaked across "+
			"the session-scope boundary this item exists to hold", line2)
	}
}
