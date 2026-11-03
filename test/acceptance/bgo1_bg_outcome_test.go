package acceptance

// BG-OUTCOME phase 1 (manager ruling 11-02 22:3x): a Bash call launched with
// run_in_background is filed Backgrounded at its post, and its end arrives
// later as a task notification, which Claude Code hands to UserPromptSubmit
// as the prompt (a turn of its own when the session is idle, a queued
// command mid-turn). The catch-up reads the notification's closed fields
// (tool-use-id, status, and the exit code at the end of the summary), keeps
// one outcome_join record (the 1.2.0 kind for a call's outcome learned after
// its execution record), and the Stop judges the run by it. The summary's
// description is the agent's text and is never kept; nor is the output-file
// path.

import (
	"strings"
	"testing"
)

// bgoCall runs a Bash call in dir launched in the background: the hook, then
// a post whose response names a background task.
func (e *env) bgoCall(dir, id string, input map[string]any) {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Bash", input, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 {
		e.t.Fatalf("hook %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	more := map[string]any{"cwd": dir, "tool_response": map[string]any{"stdout": "", "stderr": "", "interrupted": false, "backgroundTaskId": "bgo1task"}}
	if res := e.post(stPayload(e.t, "PostToolUse", id, "Bash", input, more), envs...); res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
}

// bgoNote is the notification Claude Code sends when the task of call id
// ends, with summary's tail after the description.
func bgoNote(id, status, tail string) string {
	return "<task-notification>\n<task-id>bgo1task</task-id>\n<tool-use-id>" + id + "</tool-use-id>\n" +
		"<output-file>/private/tmp/claude-501/bgo1/tasks/bgo1task.output</output-file>\n<status>" + status + "</status>\n" +
		"<summary>Background command \"Run the unit suite\" " + tail + "</summary>\n</task-notification>"
}

// bgoTurn is a trusted project with an edit, then its gate launched in the
// background, then the prompt text under promptID: the Stop line after it.
// It fails the test if the prompt printed a catch-up line for the turn.
func bgoTurn(t *testing.T, promptID, text string) (*env, string) {
	t.Helper()
	return bgoTurnIn(t, true, promptID, text)
}

// bgoTurnIn is bgoTurn with the spec trusted when trusted: an untrusted spec
// binds no declared gate, and the run is judged as its learned gate, go test.
func bgoTurnIn(t *testing.T, trusted bool, promptID, text string) (*env, string) {
	t.Helper()
	e, dir := dghEnv(t, trusted)
	e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	e.bgoCall(dir, "toolu_bg", dghRun)
	if text != "" {
		res, out := e.prompt(dir, text, promptID, stAgent)
		if res.exitCode != 0 {
			t.Fatalf("recap pending: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if strings.Contains(out.SystemMessage, "previous turn") {
			t.Errorf("a notification queued into the turn printed a catch-up line: %q", out.SystemMessage)
		}
	}
	return e, e.dghStop(dir)
}

const bgoBackground = "moved to the background"

// TestBGO1a_ABackgroundRunTakesItsEndsExitCode: the gate's background run
// ends "failed with exit code 1", and the notification arrives mid-turn (the
// turn's own prompt_id). The Stop line reads the run red, exit 1, not moved
// to the background. Premise: with no notification the line says it moved
// to the background. Break: keep no outcome_join, or join none at classify.
func TestBGO1a_ABackgroundRunTakesItsEndsExitCode(t *testing.T) {
	if _, line := bgoTurn(t, "p1", ""); !strings.Contains(line, bgoBackground) {
		t.Fatalf("premise: a background run with no end: Stop line %q, want %q", line, bgoBackground)
	}
	_, line := bgoTurn(t, "p1", bgoNote("toolu_bg", "failed", "failed with exit code 1"))
	if !strings.Contains(line, "'unit' exited 1") || strings.Contains(line, bgoBackground) {
		t.Errorf("a background run that ended with exit 1: Stop line %q, want 'unit' exited 1", line)
	}
}

// TestBGO1b_ACompletedEndIsGreen: the run ends "completed (exit code 0)",
// and the Stop line says the gate passed after the last edit.
func TestBGO1b_ACompletedEndIsGreen(t *testing.T) {
	_, line := bgoTurn(t, "p1", bgoNote("toolu_bg", "completed", "completed (exit code 0)"))
	if want := "verified - 'unit' passed after the last edit."; !strings.Contains(line, want) {
		t.Errorf("a background run that completed with exit 0: Stop line %q, want %q", line, want)
	}
}

// TestBGO1c_TheEndKeepsClosedFieldsOnly: one outcome_join, the call's id,
// the outcome the status names, the exit code and the source; no field holds
// the description, the output-file path or the task id.
func TestBGO1c_TheEndKeepsClosedFieldsOnly(t *testing.T) {
	e, _ := bgoTurn(t, "p1", bgoNote("toolu_bg", "failed", "failed with exit code 1"))
	ends := e.recordsOfType("outcome_join")
	if len(ends) != 1 {
		t.Fatalf("outcome_join records %v, want one", ends)
	}
	got := ends[0]
	if got["tool_use_id"] != "toolu_bg" || got["outcome"] != "failed" || got["exit_code"] != float64(1) ||
		got["outcome_source"] != "task_notification" {
		t.Errorf("outcome_join %v, want toolu_bg, failed, exit 1, from task_notification", got)
	}
	for k, v := range got {
		if s, ok := v.(string); ok && (strings.Contains(s, "unit suite") || strings.Contains(s, "/tasks/") || strings.Contains(s, "bgo1task")) {
			t.Errorf("outcome_join key %s holds notification text: %q", k, s)
		}
	}
}

// TestBGO1d_TheExitCodeIsTheSummarysEnd: a description that itself reads
// "failed with exit code 9" ends "failed with exit code 1", and the run
// exited 1: the code is read from the summary's end, never from the agent's
// words. Break: read the first code the summary names.
func TestBGO1d_TheExitCodeIsTheSummarysEnd(t *testing.T) {
	note := strings.Replace(bgoNote("toolu_bg", "failed", "failed with exit code 1"),
		"Run the unit suite", "rerun what failed with exit code 9", 1)
	_, line := bgoTurn(t, "p1", note)
	if !strings.Contains(line, "'unit' exited 1") {
		t.Errorf("a description naming exit code 9 before an end with exit code 1: Stop line %q, want 'unit' exited 1", line)
	}
}

// TestBGO1e_OnlyANotificationPromptIsRead: text typed while the turn runs
// that quotes a notification is not one: no outcome_join, and the run stays in the
// background. Passes today; break: read a block anywhere in the prompt.
func TestBGO1e_OnlyANotificationPromptIsRead(t *testing.T) {
	e, line := bgoTurn(t, "p1", "why did this fail?\n"+bgoNote("toolu_bg", "failed", "failed with exit code 1"))
	if ends := e.recordsOfType("outcome_join"); len(ends) != 0 {
		t.Errorf("a quoted notification kept outcome_join %v, want none", ends)
	}
	if !strings.Contains(line, bgoBackground) {
		t.Errorf("a quoted notification: Stop line %q, want %q", line, bgoBackground)
	}
}

// TestBGO1f_AnEndOfAnotherKindIsNotRead: a notification of a subagent or a
// workflow ("Agent ... finished") names no exit code, and keeps nothing.
// Passes today; break: read a status with no exit code.
func TestBGO1f_AnEndOfAnotherKindIsNotRead(t *testing.T) {
	note := strings.Replace(bgoNote("toolu_bg", "completed", "finished"), "Background command", "Agent", 1)
	e, line := bgoTurn(t, "p1", note)
	if ends := e.recordsOfType("outcome_join"); len(ends) != 0 {
		t.Errorf("an agent's end kept outcome_join %v, want none", ends)
	}
	if !strings.Contains(line, bgoBackground) {
		t.Errorf("an agent's end: Stop line %q, want %q", line, bgoBackground)
	}
}

// TestBGO1g_ALearnedGateTakesItsEndToo: with no declared gate to bind it
// (an untrusted spec), the run is its learned gate's, go test, and the end
// reaches that gate as it does a declared one: classify's gate runs and a
// declared gate's line runs are built apart. Premise: with no notification
// the line says it moved to the background. Break: join the end on one of
// the two paths only.
func TestBGO1g_ALearnedGateTakesItsEndToo(t *testing.T) {
	if _, line := bgoTurnIn(t, false, "p1", ""); !strings.Contains(line, "go test "+bgoBackground) {
		t.Fatalf("premise: an unbound background run with no end: Stop line %q, want %q", line, "go test "+bgoBackground)
	}
	_, line := bgoTurnIn(t, false, "p1", bgoNote("toolu_bg", "failed", "failed with exit code 1"))
	if !strings.Contains(line, "go test exited 1") || strings.Contains(line, bgoBackground) {
		t.Errorf("an unbound background run that ended with exit 1: Stop line %q, want go test exited 1", line)
	}
}

// TestBGO1h_AForegroundRunKeepsItsOwnOutcome: an end naming a call that ran
// in the foreground changes nothing: its post saw how it ended. A prompt that
// is only a notification naming such a call (pasted, or a stale task's) does
// not rewrite it. Break: join an end to a run not moved to the background.
func TestBGO1h_AForegroundRunKeepsItsOwnOutcome(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	e.dghCall(dir, "toolu_fg", "Bash", dghRun)
	if res, _ := e.prompt(dir, bgoNote("toolu_fg", "failed", "failed with exit code 1"), "p1", stAgent); res.exitCode != 0 {
		t.Fatalf("recap pending: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if line, want := e.dghStop(dir), "verified - 'unit' passed after the last edit."; !strings.Contains(line, want) {
		t.Errorf("a foreground run that passed, then an end naming it: Stop line %q, want %q", line, want)
	}
}
