package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// failedBash records a Bash call of command that failed with errMsg, the way
// Claude Code reports one: declared, then a PostToolUseFailure.
func failedBash(t *testing.T, e *env, toolUseID, command, errMsg string) {
	t.Helper()
	p := defaultPayload()
	p.ToolUseID = toolUseID
	p.ToolInput = map[string]any{"command": command}
	e.mustHook(p.build(t))
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUseFailure",
		"session_id":      testSession,
		"transcript_path": "/tmp/transcripts/sess-1.jsonl",
		"cwd":             "/tmp/project",
		"permission_mode": "default",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": command},
		"tool_use_id":     toolUseID,
		"error":           errMsg,
		"is_interrupt":    false,
		"duration_ms":     12,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.mustPost(string(b))
}

// Issue #52, for a missing program: a turn whose only failure is a `which` or
// `command -v` probe that found nothing, or a command the shell could not
// find (exit 127), prints nothing, and the record and the report still hold
// it. On the 100-run benchmark a missing pytest and a which probe were among
// the Haiku false alarms of the recorded-failure line, on runs whose final
// report was accurate. Break: count every failed Bash call toward the line,
// and each turn prints "1 recorded failure".
func TestH52_AMissingProgramAloneStaysSilent(t *testing.T) {
	for _, tc := range []struct{ name, command, err string }{
		{"which", "which pytest", "Exit code 1"},
		{"command -v", "command -v pytest >/dev/null 2>&1", "Exit code 1"},
		{"type", "type pytest", "Exit code 1\nbash: type: pytest: not found"},
		{"not found", "pytest -q", "Exit code 127\nbash: pytest: command not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			failedBash(t, e, "toolu_probe", tc.command, tc.err)

			if line, ok := e.recapLine(stopPayload(testSession, "All 9 tests pass.", false)); ok {
				t.Errorf("a turn whose only failure is %q printed %q", tc.command, line)
			}
			d := e.digest("--session", testSession, "--prompt", "prompt-1")
			if sf := d.SilentFailures; sf.Failed != 1 || sf.MissingPrograms != 1 || sf.Fires {
				t.Errorf("digest failed/missing_programs/fires = %d/%d/%v, want 1/1/false: the failure is recorded, not hidden",
					sf.Failed, sf.MissingPrograms, sf.Fires)
			}
			out := e.run("", nil, "report", "--session", testSession).stdout
			if !strings.Contains(out, "failed calls: 1\n") || !strings.Contains(out, "    missing programs among them: 1 (") {
				t.Errorf("the report does not show the missing program:\n%s", out)
			}
		})
	}
}

// Issue #52, beside a real failure: the line fires on the failing build alone
// and names what it set aside, so its numbers add up to the report's failed
// calls. Break: let the probe mask the build, count it, or drop the clause,
// and this reads nothing, "2 recorded failures" or a bare "1 recorded
// failure.".
func TestH52_AMissingProgramBesideARealFailureIsNamed(t *testing.T) {
	for _, tc := range []struct {
		lookup bool
		want   string
	}{
		{false, "rashomon: 1 recorded failure, besides 1 missing program.\n"},
		{true, "rashomon: 1 recorded failure, besides 1 failed lookup and 1 missing program.\n"},
	} {
		e := newEnv(t)
		e.watched(testSession)
		if tc.lookup {
			readOfADirectory(t, e, "toolu_read")
		}
		failedBash(t, e, "toolu_which", "which pytest", "Exit code 1")
		failedBash(t, e, "toolu_make", "make test", "Exit code 2")

		line, ok := e.recapLine(stopPayload(testSession, "All 9 tests pass.", false))
		if !ok || !strings.Contains(line, tc.want) {
			t.Errorf("line = %q (printed %v), want %q", line, ok, tc.want)
		}
	}
}

// A probe on a line with another command is no probe: `which pytest &&
// pytest -q` exits 1 when pytest ran and a test failed, so the failure counts.
// Control: passes on main 6701e40b, which counts every failed Bash call.
// Break: read a probe off the line's program alone.
func TestH52_AProbeBesideAnotherCommandStillCounts(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	failedBash(t, e, "toolu_both", "which pytest && pytest -q", "Exit code 1")

	line, ok := e.recapLine(stopPayload(testSession, "All 9 tests pass.", false))
	if !ok || !strings.Contains(line, "rashomon: 1 recorded failure.\n") {
		t.Errorf("line = %q (printed %v), want a bare \"1 recorded failure.\"", line, ok)
	}
}

// TestH52_TheDeclarationRecordsAProbe: the declaration of a line of nothing
// but probes carries probe, true, and a line with another command omits the
// key, as launch is omitted (call schema 10). The bit is read from the words
// at the hook and the words dropped, so a record that holds no command, as
// the hook writes none, still sets the probe aside. Break: drop the key,
// write it false, or set it on a probe beside another command.
func TestH52_TheDeclarationRecordsAProbe(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	failedBash(t, e, "toolu_which", "which jq; type -P yq", "Exit code 1")
	failedBash(t, e, "toolu_both", "which pytest && pytest -q", "Exit code 1")
	var got []any
	for _, d := range e.declarations(testSession) {
		v, ok := d.fields["probe"]
		if !ok {
			v = "absent"
		}
		got = append(got, v)
		if sv := d.fields["schema_version"]; sv != float64(10) {
			t.Errorf("declaration written at schema_version %v, want 10", sv)
		}
	}
	if len(got) != 2 || got[0] != true || got[1] != "absent" {
		t.Errorf("probe per Bash declaration %v, want [true absent]", got)
	}
}
