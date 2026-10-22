package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-150 -- under Codex, the Stop entry watch installs prints the end-of-turn
// line for a turn with a failed call the agent did not mention, and nothing
// for a clean turn; nothing ever goes where Codex would give it to the model.
//
// The line reaches the user as a JSON systemMessage on a Stop hook's stdout,
// which Codex shows as a warning. Anything else on that stdout is what the
// item holds down: a key that steers the turn (decision, continue, reason)
// would make Codex continue the agent with our words as its prompt. The
// UserPromptSubmit catch-up is the other way in -- Codex adds a hook's plain
// stdout there to the model's context -- so under Codex it prints nothing.
//
// The calls are phase 4's second session, replayed as in H-149 against its
// real rollout lines; the turn with python3 missing.py failed with exit 2,
// which only the rollout says. Each Stop runs the command line watch wrote,
// through a shell, as Codex runs it.
func TestH150_CodexStopSaysWhenATurnFailedAndOnlyThen(t *testing.T) {
	e := newEnv(t)
	e.makeCodexDir("")
	mustWatchCodex(t, e)
	var stop string
	for _, g := range ours(e.codexHooks()["Stop"]) {
		stop = g.Hooks[0].Command
	}
	if !strings.Contains(stop, " recap --harness codex --install ") {
		t.Fatalf("watch --harness codex installed no Stop entry running recap: %q", stop)
	}

	rollout := filepath.Join(t.TempDir(), "rollout-"+p4Session2+".jsonl")
	if err := os.WriteFile(rollout, mustRead(t, filepath.Join(moduleRoot, "test", "fixtures", "codex", "phase4", "rollout-"+p4Session2+".jsonl")), 0o600); err != nil {
		t.Fatal(err)
	}
	rollouts := map[string]string{p4Session2: rollout}
	stopPayload := func(turn, message string) string {
		b, err := json.Marshal(map[string]any{
			"session_id": p4Session2, "turn_id": turn, "hook_event_name": "Stop",
			"cwd": "/home/user/rashomon-phase4/work", "model": "gpt-6-luna", "permission_mode": "default",
			"transcript_path": rollout, "stop_hook_active": false, "last_assistant_message": message,
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	byID := map[string]codexCall{}
	for _, c := range phase4Calls {
		byID[c.id] = c
	}

	// A clean turn: python3 calc.py, exit 0.
	replayCodex(t, e, []codexCall{byID[p4Calc]}, rollouts)
	res := e.sh(stop, stopPayload("01a1645b-577f-74c3-875e-a0bc6b7551a3", "It printed 5."))
	if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
		t.Errorf("clean turn: exit %d, stdout %q, stderr %q; want 0 and silence", res.exitCode, res.stdout, res.stderr)
	}

	// A failed call the final message does not mention.
	replayCodex(t, e, []codexCall{byID[p4Missing]}, rollouts)
	res = e.sh(stop, stopPayload("01a1645c-1f03-7453-9e64-c84df3239413", "Done."))
	if res.exitCode != 0 || res.stderr != "" {
		t.Fatalf("failed turn: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("failed turn: stdout is not one JSON object: %v\n%q", err, res.stdout)
	}
	msg, _ := out["systemMessage"].(string)
	if len(out) != 1 || msg == "" {
		t.Fatalf("failed turn: stdout %q; want a systemMessage and no other key", res.stdout)
	}
	lines := strings.Split(msg, "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "rashomon: 1 recorded failure.") ||
		strings.TrimSpace(lines[1]) != "→ rashomon report --session "+p4Session2 {
		t.Errorf("failed turn: the line reads\n%s", msg)
	}
	// The permanent Codex coverage reason is set aside; nothing else is said.
	if strings.Contains(msg, "unverified") || strings.Contains(msg, "hook_entry_unresolved") {
		t.Errorf("the line speaks of Codex's permanent coverage reason:\n%s", msg)
	}

	// The same turn's Stop again says nothing: one line per turn.
	if res := e.sh(stop, stopPayload("01a1645c-1f03-7453-9e64-c84df3239413", "Done.")); res.stdout != "" {
		t.Errorf("a second Stop for the turn printed %q", res.stdout)
	}

	// The UserPromptSubmit catch-up, run by hand since watch installs none.
	// Its turn has something recap would say with no final message to read:
	// a call that never got its PostToolUse, as an interrupted turn leaves
	// one. Under Codex it prints nothing all the same.
	const interrupted = "01a1645d-0000-7000-8000-000000000001"
	pre, err := json.Marshal(map[string]any{
		"session_id": p4Session2, "turn_id": interrupted, "hook_event_name": "PreToolUse",
		"cwd": "/home/user/rashomon-phase4/work", "model": "gpt-6-luna", "permission_mode": "default",
		"transcript_path": rollout, "tool_name": "Bash", "tool_use_id": "exec-interrupted",
		"tool_input": map[string]any{"command": "sleep 600"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res := e.run(string(pre), nil, append([]string{"hook", "--harness", "codex"}, e.installArgs()...)...); res.exitCode != 0 {
		t.Fatalf("PreToolUse: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	pending := strings.Replace(stop, " recap ", " recap pending ", 1)
	ups, err := json.Marshal(map[string]any{
		"session_id": p4Session2, "turn_id": "01a1645d-0000-7000-8000-000000000002", "hook_event_name": "UserPromptSubmit",
		"cwd": "/home/user/rashomon-phase4/work", "model": "gpt-6-luna", "permission_mode": "default",
		"transcript_path": rollout, "prompt": "next",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res := e.sh(pending, string(ups)); res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
		t.Errorf("recap pending under Codex: exit %d, stdout %q, stderr %q; want 0 and silence", res.exitCode, res.stdout, res.stderr)
	}
	// Its twin: the same turn's Stop does speak, so the silence above is the
	// pending path's, not a turn with nothing to say.
	if res := e.sh(stop, stopPayload(interrupted, "Stopped.")); !strings.Contains(res.stdout, "1 declaration without recorded execution") {
		t.Errorf("Stop for the interrupted turn: stdout %q; want the line about the call without an execution", res.stdout)
	}
}

// TestH150_TheAlertDoesNotFireWhenTheAgentSaidSo: the line is about a failure
// the final message left out, as under Claude Code. A turn whose message names
// the failure prints nothing.
func TestH150_TheAlertDoesNotFireWhenTheAgentSaidSo(t *testing.T) {
	e := newEnv(t)
	e.makeCodexDir("")
	mustWatchCodex(t, e)
	stop := ours(e.codexHooks()["Stop"])[0].Hooks[0].Command
	rollout := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(rollout, mustRead(t, filepath.Join(moduleRoot, "test", "fixtures", "codex", "phase4", "rollout-"+p4Session2+".jsonl")), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range phase4Calls {
		if c.id == p4Missing {
			replayCodex(t, e, []codexCall{c}, map[string]string{p4Session2: rollout})
		}
	}
	b, _ := json.Marshal(map[string]any{
		"session_id": p4Session2, "turn_id": "01a1645c-1f03-7453-9e64-c84df3239413", "hook_event_name": "Stop",
		"cwd": "/home/user/rashomon-phase4/work", "model": "gpt-6-luna", "permission_mode": "default",
		"transcript_path": rollout, "stop_hook_active": false,
		"last_assistant_message": "python3 missing.py failed: the file does not exist.",
	})
	if res := e.sh(stop, string(b)); res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
		t.Errorf("acknowledged failure: exit %d, stdout %q, stderr %q; want 0 and silence", res.exitCode, res.stdout, res.stderr)
	}
}
