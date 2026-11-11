package acceptance

// H-167: the stale-gate note at a call is raised by the first edit after a
// green run, and a failed Edit is no edit. Under a trusted spec gate (D222,
// as DGH-3), go test passes, an Edit fails (old string not found), then an
// Edit applies. The failed Edit's post tells nobody it wrote anything; the
// Edit that applied is the call that left the green behind, and its post
// tells the model so. Before the rule, the failed Edit was the first write
// after the green, so the Edit that applied was "a write after it" and said
// nothing: the model was never told the gate it ran no longer covered the
// file. The risk it holds down is a reader of the writes that counts a call
// the tool reported it did not apply, here the one the note is raised by
// (staleHere). Its twin is the same run without the failed Edit, whose note
// must read the same, so the note is not trivially present.

import (
	"testing"
)

// staleNote is the post output of an Edit that left the green run of 'unit'
// behind, 2 minutes after it.
const staleNote = `{"hookSpecificOutput":{"additionalContext":"rashomon record: gate 'unit' exited 0 earlier in this session (2 min ago); this call then wrote 'internal/a.go'. No later run of the gate is recorded.","hookEventName":"PostToolUse"},"systemMessage":"※ rashomon told the agent: \"rashomon record: gate 'unit' exited 0 earlier in this session (2 min ago); this call then wrote 'internal/a.go'. No later run of the gate is recorded.\""}
`

// failedEdit runs one Edit of the session in dir that fails as Claude Code
// reports an old string not found, hook then failure post, and returns the
// post's stdout.
func (e *env) failedEdit(dir, id string) string {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	input := dghEdit(dir)
	input["old_string"] = "x"
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Edit", input, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	const msg = "String to replace not found in file."
	res := e.post(stPayload(e.t, "PostToolUseFailure", id, "Edit", input,
		map[string]any{"cwd": dir, "error": msg, "is_interrupt": false, "tool_response": msg}), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

func TestH167_TheEditThatAppliedRaisesTheStaleNote(t *testing.T) {
	e, dir := dghEnv(t, true)
	if out := e.dghCall(dir, "toolu_g", "Bash", dghRun); out != "" {
		t.Errorf("the green run printed %q, want nothing", out)
	}
	e.backdate("toolu_g", 120_000)
	if out := e.failedEdit(dir, "toolu_f"); out != "" {
		t.Errorf("the failed Edit's post printed %q, want nothing: it wrote no file", out)
	}
	if out := e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir)); out != staleNote {
		t.Errorf("the Edit that applied after the failed one printed\n%q\nwant\n%q", out, staleNote)
	}
}

// TestH167_TwinWithoutTheFailedEdit: the same run with no failed Edit before
// the one that applied gives the same note, so the note above is the Edit's
// own and not a second reading.
func TestH167_TwinWithoutTheFailedEdit(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.dghCall(dir, "toolu_g", "Bash", dghRun)
	e.backdate("toolu_g", 120_000)
	if out := e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir)); out != staleNote {
		t.Errorf("the Edit after the green run printed\n%q\nwant\n%q", out, staleNote)
	}
}
