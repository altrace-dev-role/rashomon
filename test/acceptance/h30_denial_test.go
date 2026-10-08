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
	if !strings.Contains(out, "denied before running: "+p.ToolUseID) {
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

// pathDeniedText is what Claude Code 2.1.292 returns when a Read, Edit or Write is
// refused by a settings deny rule on its path, verbatim from all five such
// refusals in a 100-run benchmark.
const pathDeniedText = "<tool_use_error>File is in a directory that is denied by your " +
	"permission settings.</tool_use_error>"

// writeTwoCallTranscript writes the shape the benchmark recorded: one call that
// ran and returned, then an Edit answered by the given error text.
func writeTwoCallTranscript(t *testing.T, e *env, ranID, editID, editText string) string {
	t.Helper()
	path := filepath.Join(e.home, "path-denial-transcript.jsonl")
	use := func(id, name string) map[string]any {
		return map[string]any{"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]any{{"type": "tool_use", "id": id, "name": name}},
		}}
	}
	result := func(id, text string, isError bool) map[string]any {
		return map[string]any{"message": map[string]any{
			"role": "user",
			"content": []map[string]any{{
				"type": "tool_result", "tool_use_id": id, "is_error": isError, "content": text,
			}},
		}}
	}
	var buf []byte
	for _, l := range []map[string]any{
		use(ranID, "Bash"), result(ranID, "ok", false),
		use(editID, "Edit"), result(editID, editText, true),
	} {
		body, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(append(buf, body...), '\n')
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestH30_AnEditDeniedByAPathRuleIsNotACoverageFailure is the benchmark's
// shape, end to end. Claude Code refuses a path-denied Edit while checking its
// input, BEFORE PreToolUse, so the call is in the transcript and in neither
// the declarations nor the executions. In all five benchmark runs that read as
// missing-from-store AND executed-but-unrecorded, and a session where the
// agent did exactly the right thing was reported unverified.
func TestH30_AnEditDeniedByAPathRuleIsNotACoverageFailure(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	const editID = "toolu_path_denied"
	p := defaultPayload()
	p.TranscriptPath = writeTwoCallTranscript(t, e, p.ToolUseID, editID, pathDeniedText)
	e.mustHook(p.build(t))
	e.postIDs(p.TranscriptPath, p.ToolUseID)
	// No hook of either kind for editID: neither fires for this refusal.
	e.probe("end", testSession)

	rep := e.report(testSession)
	if len(rep.Transcripts) != 1 {
		t.Fatalf("want one transcript group, got %d", len(rep.Transcripts))
	}
	tr := rep.Transcripts[0]
	if len(tr.DeniedByUser) != 1 || tr.DeniedByUser[0] != editID {
		t.Errorf("denied_by_user = %v, want [%s]", tr.DeniedByUser, editID)
	}
	if len(tr.ExecutedButUnrecorded) != 0 {
		t.Errorf("executed_but_unrecorded = %v; the Edit was refused and never ran",
			tr.ExecutedButUnrecorded)
	}
	if len(tr.MissingFromStore) != 0 {
		t.Errorf("missing_from_store = %v; Claude Code fires no hook for a path-denied "+
			"Edit, so its absence from the store is not a recorder gap", tr.MissingFromStore)
	}
	if len(tr.RefusedBeforeHooks) != 1 || tr.RefusedBeforeHooks[0] != editID {
		t.Errorf("refused_before_hooks = %v, want [%s]: excused from the store, it must "+
			"still be named, or ids_in_transcript exceeds ids_recorded with nothing saying why",
			tr.RefusedBeforeHooks, editID)
	}
	if rep.Coverage.State != "verified" || len(rep.Coverage.Reasons) != 0 {
		t.Errorf("coverage = %s %v, want verified with no reasons",
			rep.Coverage.State, rep.Coverage.Reasons)
	}
}

// TestH30_AnUndeclaredPromptDenialIsStillMissingFromStore holds the exemption
// to the one refusal measured to come before PreToolUse. The prompt's denial
// comes after it, so the same undeclared shape there IS a declaration the
// recorder lost, and stays a transcript mismatch.
func TestH30_AnUndeclaredPromptDenialIsStillMissingFromStore(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	const editID = "toolu_prompt_denied"
	p := defaultPayload()
	p.TranscriptPath = writeTwoCallTranscript(t, e, p.ToolUseID, editID, deniedText)
	e.mustHook(p.build(t))
	e.postIDs(p.TranscriptPath, p.ToolUseID)
	e.probe("end", testSession)

	rep := e.report(testSession)
	if len(rep.Transcripts) != 1 {
		t.Fatalf("want one transcript group, got %d", len(rep.Transcripts))
	}
	tr := rep.Transcripts[0]
	if len(tr.MissingFromStore) != 1 || tr.MissingFromStore[0] != editID {
		t.Errorf("missing_from_store = %v, want [%s]: PreToolUse fires before the prompt, "+
			"so an undeclared prompt denial is a lost declaration", tr.MissingFromStore, editID)
	}
	if len(tr.RefusedBeforeHooks) != 0 {
		t.Errorf("refused_before_hooks = %v; a prompt denial comes after PreToolUse", tr.RefusedBeforeHooks)
	}
}

// inputCheckTexts are Edit and Write mistakes Claude Code refuses while
// checking the call's input, verbatim from 2.1.280. As with the path refusal,
// no hook fires for them; unlike it, they are not denials.
var inputCheckTexts = map[string]string{
	"old_string not in the file": "<tool_use_error>String to replace not found in file.\n" +
		"String: zzz</tool_use_error>",
	"file not read first": "<tool_use_error>File has not been read yet. Read it first before " +
		"writing to it.</tool_use_error>",
}

// TestH30_AnEditRefusedWhileItsInputWasCheckedIsNotACoverageFailure is the
// class the path refusal belongs to. Claude Code checks an Edit's input before
// PreToolUse, and when the check fails no hook fires. An ordinary mistake -- an
// old_string that is not in the file, a file not read first -- then read as
// missing-from-store AND executed-but-unrecorded, and the session went
// unverified for a call that never ran.
func TestH30_AnEditRefusedWhileItsInputWasCheckedIsNotACoverageFailure(t *testing.T) {
	for name, text := range inputCheckTexts {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			const editID = "toolu_input_refused"
			p := defaultPayload()
			p.TranscriptPath = writeTwoCallTranscript(t, e, p.ToolUseID, editID, text)
			e.mustHook(p.build(t))
			e.postIDs(p.TranscriptPath, p.ToolUseID)
			// No hook of either kind for editID: the input check refused it first.
			e.probe("end", testSession)

			rep := e.report(testSession)
			if len(rep.Transcripts) != 1 {
				t.Fatalf("want one transcript group, got %d", len(rep.Transcripts))
			}
			tr := rep.Transcripts[0]
			if len(tr.RefusedBeforeHooks) != 1 || tr.RefusedBeforeHooks[0] != editID {
				t.Errorf("refused_before_hooks = %v, want [%s]", tr.RefusedBeforeHooks, editID)
			}
			if len(tr.MissingFromStore) != 0 || len(tr.ExecutedButUnrecorded) != 0 {
				t.Errorf("missing_from_store = %v, executed_but_unrecorded = %v; the Edit never "+
					"ran and no hook could record it", tr.MissingFromStore, tr.ExecutedButUnrecorded)
			}
			if len(tr.DeniedByUser) != 0 {
				t.Errorf("denied_by_user = %v; a failed input check is not a refusal", tr.DeniedByUser)
			}
			if rep.Coverage.State != "verified" || len(rep.Coverage.Reasons) != 0 {
				t.Errorf("coverage = %s %v, want verified with no reasons",
					rep.Coverage.State, rep.Coverage.Reasons)
			}
			out := e.run("", nil, "report", "--session", testSession).stdout
			if !strings.Contains(out, "refused before any hook: "+editID) {
				t.Errorf("the render does not name the refused call:\n%s", out)
			}
		})
	}
}
