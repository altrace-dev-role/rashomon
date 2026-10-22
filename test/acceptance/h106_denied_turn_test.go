package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// H-106 -- a turn a refused permission prompt interrupted still gets its line.
//
// Saying No at a permission prompt interrupts the turn, and Claude Code fires
// no Stop after an interrupt. The recap ran only on Stop, so the refused call
// -- a declaration with no execution, the case the line most exists for --
// was the one case it could never show. Found using the product: a real
// session refused a curl, the store named it "denied", and no line appeared.
//
// The catch-up runs on UserPromptSubmit, before the next prompt makes a call,
// so the newest turn in the store is the one that just ended. It speaks only
// when no recap reached that turn, so a turn Stop already handled is never
// reported twice.
//
// Break: drop the UserPromptSubmit path and the refused call is silent forever.

// recapPending runs the UserPromptSubmit catch-up as watch installs it, and
// reports the line it printed, if any. Plain stdout on UserPromptSubmit is
// injected into the model's context, so anything but empty output or the JSON
// envelope fails the test outright.
func (e *env) recapPending(sessionID, nextPrompt string) (string, bool) {
	e.t.Helper()
	payload := fmt.Sprintf(`{"session_id":%q,"hook_event_name":"UserPromptSubmit","prompt":%q}`,
		sessionID, nextPrompt)
	res := e.run(payload, nil, append([]string{"recap", "pending"}, e.installArgs()...)...)
	if res.exitCode != 0 {
		// Exit 2 on UserPromptSubmit erases the user's prompt.
		e.t.Fatalf("recap pending: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res.stdout == "" {
		return "", false
	}
	var out recapOutput
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		e.t.Fatalf("recap pending wrote plain stdout, which UserPromptSubmit would inject "+
			"into the model's context: %v\n%s", err, res.stdout)
	}
	return out.SystemMessage, true
}

func TestH106_ARefusedTurnIsReportedWhenTheNextPromptIsSent(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.PromptID = "prompt-1"
	e.mustHook(p.build(t))
	// No post and no Stop: the user said No, the call never ran, and the
	// interrupt meant Stop never fired.

	line, ok := e.recapPending(testSession, "try something else")
	if !ok {
		t.Fatal("the next prompt printed nothing for a turn whose only call was refused")
	}
	if !strings.Contains(line, "previous turn") || !strings.Contains(line, "without recorded execution") {
		t.Errorf("line = %q, want it marked as the previous turn and naming the call "+
			"without a recorded execution", line)
	}

	if again, ok := e.recapPending(testSession, "and another thing"); ok {
		t.Errorf("a second prompt reported the same turn again: %q", again)
	}
}

func TestH106_ATurnStopAlreadyCheckedIsNotReportedAgain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		finding bool
	}{
		{"Stop printed a line", true},
		{"Stop found nothing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			p := defaultPayload()
			p.PromptID = "prompt-1"
			e.mustHook(p.build(t))
			if tc.finding {
				e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))
			} else {
				e.mustPost(defaultPost().build(t))
			}
			e.recapLine(stopPayload(testSession, "Ran it.", false))

			if line, ok := e.recapPending(testSession, "next"); ok {
				t.Errorf("Stop already checked this turn, and the next prompt reported it "+
					"again: %q", line)
			}
		})
	}
}

// TestH106_StopsDecisionStandsForATurnItReached pins what the bookkeeping is
// for. Without it, re-evaluating a turn Stop already checked is harmless only
// while the record is unchanged; a call recorded after Stop (a backgrounded
// shell's declaration landing late) would make the catch-up print a second
// verdict on a turn Stop already ruled on. The catch-up is for turns Stop never
// reached, and only those.
func TestH106_StopsDecisionStandsForATurnItReached(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.PromptID = "prompt-1"
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))
	if line, ok := e.recapLine(stopPayload(testSession, "Ran it.", false)); ok {
		t.Fatalf("a clean turn printed at Stop: %q", line)
	}

	late := defaultPayload()
	late.PromptID = "prompt-1"
	late.ToolUseID = "toolu_late"
	e.mustHook(late.build(t))

	if line, ok := e.recapPending(testSession, "next"); ok {
		t.Errorf("the catch-up re-ruled on a turn Stop already checked: %q", line)
	}
}
