package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-30 -- a denied call is a user decision, not a coverage failure.
//
// Field-reported from a real `claude -p` session: a call the user DENIED showed
// up in executed_but_unrecorded and put execution_mismatch into the coverage
// reasons. That list exists to say the PostToolUse recorder did not fire, so
// the user exercising the permission prompt was being reported as the tool
// being broken -- and coverage, the one number a reader trusts, was downgraded
// by it.
//
// These tests drive the real binary end to end, because the unit tests for the
// detector cannot show the consequence: the wrong classification only becomes a
// coverage reason three layers up.

const deniedText = "The user doesn't want to proceed with this tool use. The tool use was " +
	"rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP " +
	"what you are doing and wait for the user to tell you how to proceed."

// writeResultTranscript writes a transcript in which one declared call is
// answered by a tool_result carrying the given text and error flag.
func writeResultTranscript(t *testing.T, e *env, toolUseID, text string, isError bool) string {
	t.Helper()
	path := filepath.Join(e.home, "denial-transcript.jsonl")

	lines := []map[string]any{
		{"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]any{{"type": "tool_use", "id": toolUseID, "name": "Bash"}},
		}},
		{"message": map[string]any{
			"role": "user",
			"content": []map[string]any{{
				"type": "tool_result", "tool_use_id": toolUseID,
				"is_error": isError, "content": text,
			}},
		}},
		{"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]any{{"type": "text", "text": "I stopped as asked."}},
		}},
	}

	var buf []byte
	for _, l := range lines {
		body, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(buf, body...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestH30_ADeniedCallIsNotACoverageFailure is the bug, end to end. The call is
// DECLARED (PreToolUse fired), never EXECUTED (it did not run), and answered in
// the transcript by a denial.
func TestH30_ADeniedCallIsNotACoverageFailure(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.TranscriptPath = writeResultTranscript(t, e, p.ToolUseID, deniedText, true)
	e.mustHook(p.build(t))
	// No post hook on purpose: a denied call never runs, so PostToolUse never
	// fires. That absence is the whole shape of a denial in the store.
	e.probe("end", testSession)

	rep := e.report(testSession)
	if len(rep.Transcripts) != 1 {
		t.Fatalf("want one transcript group, got %d", len(rep.Transcripts))
	}
	tr := rep.Transcripts[0]

	if len(tr.ExecutedButUnrecorded) != 0 {
		t.Errorf("executed_but_unrecorded = %v. A denied call did not execute, so listing it "+
			"as executed-but-unrecorded reports the recorder as broken when the user simply "+
			"said no.", tr.ExecutedButUnrecorded)
	}
	if len(tr.DeniedByUser) != 1 || tr.DeniedByUser[0] != p.ToolUseID {
		t.Errorf("denied_by_user = %v, want [%s]", tr.DeniedByUser, p.ToolUseID)
	}
	for _, r := range rep.Coverage.Reasons {
		if r == "execution_mismatch" {
			t.Errorf("coverage carries execution_mismatch because of a denial. Coverage is the "+
				"one number a reader trusts; a user exercising the permission prompt must not "+
				"downgrade it. Reasons: %v", rep.Coverage.Reasons)
		}
	}

	out := e.run("", nil, "report", "--session", testSession).stdout
	if !strings.Contains(out, "denied by user: "+p.ToolUseID) {
		t.Errorf("the render does not name the denial:\n%s", out)
	}
}

// TestH30_AFailureIsStillACoverageFailure is the guard against the fix being a
// blanket excuse. A command that RAN and failed, with no execution record, is
// exactly what executed_but_unrecorded exists for, and the fix must not have
// swallowed it.
//
// The fixture is the adversarial one: a genuine failure whose OUTPUT QUOTES the
// denial sentence. Matching that text anywhere, rather than anchored and beside
// is_error, would classify a real execution as a user decision -- the same bug
// as the one being fixed, pointing the other way.
func TestH30_AFailureIsStillACoverageFailure(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.TranscriptPath = writeResultTranscript(t, e,
		p.ToolUseID, "Exit code 1\ngrep matched: "+deniedText, true)
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	rep := e.report(testSession)
	tr := rep.Transcripts[0]

	if len(tr.DeniedByUser) != 0 {
		t.Errorf("denied_by_user = %v for a command that RAN and failed while quoting the "+
			"denial sentence. Hiding a real execution behind a user decision is the same "+
			"defect as the one this fixes.", tr.DeniedByUser)
	}
	if len(tr.ExecutedButUnrecorded) != 1 {
		t.Errorf("executed_but_unrecorded = %v, want the one failed call: it finished, and no "+
			"execution record says so", tr.ExecutedButUnrecorded)
	}
	var sawMismatch bool
	for _, r := range rep.Coverage.Reasons {
		if r == "execution_mismatch" {
			sawMismatch = true
		}
	}
	if !sawMismatch {
		t.Errorf("coverage does not carry execution_mismatch for a genuine recording gap; "+
			"reasons: %v", rep.Coverage.Reasons)
	}
}
