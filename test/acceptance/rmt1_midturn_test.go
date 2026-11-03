package acceptance

// REC-MIDTURN1 (manager ruling 11-02 22:5x): a turn's Stop line speaks for
// the whole turn once, whatever arrives while it runs. Claude Code fires
// UserPromptSubmit for a prompt it takes into a running turn (a peer message,
// a task notification, text typed while busy: a queued_command), and the
// prompt keeps the running turn's prompt_id. The catch-up read the newest
// turn in the store as one that had ended: it printed the running turn's
// line as the previous turn's and claimed it, so the turn's own Stop stayed
// silent. Found live on 11-02: recap.json held checked = spoken = the id of
// a turn that was still running, with five queued prompts in it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recapPendingIn is recapPending with the payload's prompt_id, the turn the
// UserPromptSubmit belongs to.
func (e *env) recapPendingIn(promptID, text string) string {
	e.t.Helper()
	payload := fmt.Sprintf(`{"session_id":%q,"hook_event_name":"UserPromptSubmit","prompt":%q,"prompt_id":%q}`,
		testSession, text, promptID)
	res := e.run(payload, nil, append([]string{"recap", "pending"}, e.installArgs()...)...)
	if res.exitCode != 0 {
		e.t.Fatalf("recap pending: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res.stdout == "" {
		return ""
	}
	var out recapOutput
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		e.t.Fatalf("recap pending wrote plain stdout: %v\n%s", err, res.stdout)
	}
	return out.SystemMessage
}

// recapState is recap.json's checked and spoken prompt for the session, ""
// for none.
func (e *env) recapState() (checked, spoken string) {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "recap.json"))
	if os.IsNotExist(err) {
		return "", ""
	}
	if err != nil {
		e.t.Fatal(err)
	}
	var s struct {
		Checked map[string]string `json:"checked"`
		Spoken  map[string]string `json:"spoken"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		e.t.Fatalf("recap.json: %v\n%s", err, b)
	}
	return s.Checked[testSession], s.Spoken[testSession]
}

// rmt1Turn is a watched session whose turn prompt-1 has a call in flight:
// its PreToolUse landed and its post has not.
func rmt1Turn(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.PromptID = "prompt-1"
	e.mustHook(p.build(t))
	return e
}

const rmt1Queued = "<task-notification>\n<task-id>b1</task-id>\n<status>completed</status>\n</task-notification>"

// TestRMT1a_AQueuedPromptPrintsNoCatchUpForItsOwnTurn: a prompt queued into
// turn prompt-1 while a call runs prints no line and leaves recap.json's
// checked and spoken for the session as they were. Today it prints the
// running turn's line as the previous turn's (the in-flight call "without
// recorded execution") and claims prompt-1. Break: drop the guard.
func TestRMT1a_AQueuedPromptPrintsNoCatchUpForItsOwnTurn(t *testing.T) {
	e := rmt1Turn(t)
	checked, spoken := e.recapState()
	if line := e.recapPendingIn("prompt-1", rmt1Queued); line != "" {
		t.Errorf("a prompt queued into the running turn printed %q, want no catch-up line", line)
	}
	if c, s := e.recapState(); c != checked || s != spoken {
		t.Errorf("recap.json checked/spoken %q/%q after a queued prompt, want them unchanged (%q/%q)", c, s, checked, spoken)
	}
}

// TestRMT1b_TheTurnsStopSpeaksAfterAQueuedPrompt: the call's post lands red
// after the queued prompt, and the turn's Stop prints the turn's line. Today
// the queued prompt claimed the turn, so the Stop is silent as a duplicate.
// Premise: the same turn with no queued prompt prints at its Stop.
func TestRMT1b_TheTurnsStopSpeaksAfterAQueuedPrompt(t *testing.T) {
	for _, queued := range []bool{false, true} {
		e := rmt1Turn(t)
		if queued {
			e.recapPendingIn("prompt-1", rmt1Queued)
		}
		e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))
		line, _ := e.recapLine(stopPayload(testSession, "Ran it.", false))
		switch {
		case !queued && line == "":
			t.Fatal("premise: a turn whose call failed printed nothing at its Stop")
		case queued && line == "":
			t.Error("the turn's Stop printed nothing after a prompt queued into the turn, want the turn's line")
		}
	}
}

// TestRMT1c_ANewTurnsPromptStillCatchesUp: the catch-up's own case stands.
// A turn a refused call interrupted (no post, no Stop), then the next turn's
// prompt with its own id: the line prints, marked as the previous turn.
// Break: a guard that silences every catch-up.
func TestRMT1c_ANewTurnsPromptStillCatchesUp(t *testing.T) {
	e := rmt1Turn(t)
	line := e.recapPendingIn("prompt-2", "try something else")
	if !strings.Contains(line, "previous turn") || !strings.Contains(line, "without recorded execution") {
		t.Errorf("the next turn's prompt printed %q, want the interrupted turn's line marked as the previous turn", line)
	}
}

// TestRMT1d_APromptWithNoIDKeepsTodaysCatchUp pins scope (b), NOT FIXED: a
// payload with no prompt_id (an older CLI) cannot tell a queued prompt from
// the next turn's, so a prompt queued into the running turn still prints
// that turn's line, as today. Break: read a missing id as the running turn's.
func TestRMT1d_APromptWithNoIDKeepsTodaysCatchUp(t *testing.T) {
	e := rmt1Turn(t)
	if line := e.recapPendingIn("", rmt1Queued); !strings.Contains(line, "previous turn") {
		t.Errorf("a prompt with no prompt_id printed %q, want today's catch-up line", line)
	}
}
