package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// readOfADirectory records a Read the way Claude Code reports one of a
// directory: declared, then a PostToolUseFailure carrying EISDIR.
func readOfADirectory(t *testing.T, e *env, toolUseID string) {
	t.Helper()
	p := defaultPayload()
	p.ToolUseID = toolUseID
	p.ToolName = "Read"
	p.ToolInput = map[string]any{"file_path": "/tmp/project/src"}
	e.mustHook(p.build(t))
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUseFailure",
		"session_id":      testSession,
		"transcript_path": "/tmp/transcripts/sess-1.jsonl",
		"cwd":             "/tmp/project",
		"permission_mode": "default",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": "/tmp/project/src"},
		"tool_use_id":     toolUseID,
		"error":           "EISDIR: illegal operation on a directory, read",
		"is_interrupt":    false,
		"duration_ms":     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.mustPost(string(b))
}

// H-91, for a failed lookup -- a turn whose only failure is a Read of a
// directory prints nothing, and the record and the report still hold it.
//
// Measured on a 100-run benchmark: 5 of 7 Haiku false alarms of the
// recorded-failure line rested on one Read of a directory (EISDIR) and
// nothing else, under true reports ("all 9 tests pass"). Break: count every
// failed call toward the line, and this turn prints "1 recorded failure".
func TestH91_AFailedLookupAloneStaysSilent(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	readOfADirectory(t, e, "toolu_read")

	if line, ok := e.recapLine(stopPayload(testSession, "All 9 tests pass.", false)); ok {
		t.Errorf("a turn whose only failure is a Read of a directory printed %q", line)
	}

	d := e.digest("--session", testSession, "--prompt", "prompt-1")
	if d.SilentFailures.Failed != 1 || d.SilentFailures.FailedLookups != 1 || d.SilentFailures.Fires {
		t.Errorf("digest failed/failed_lookups/fires = %d/%d/%v, want 1/1/false: the failure is recorded, not hidden",
			d.SilentFailures.Failed, d.SilentFailures.FailedLookups, d.SilentFailures.Fires)
	}
	if out := e.run("", nil, "report", "--session", testSession).stdout; !strings.Contains(out, "failed calls: 1\n") ||
		!strings.Contains(out, "    lookups among them: 1 (Read, Glob, Grep or NotebookRead, whatever the error: a directory, a missing file or folder, a bad pattern), not set against the final message\n") {
		t.Errorf("the report does not show the failed lookup:\n%s", out)
	}
}

// H-91, a failed lookup beside a real failure: the line fires on the real one,
// counts it alone, and says how many lookups it set aside, so its numbers add
// up to the report's "failed calls: 2". With no lookup the sentence is the
// plain one. Break: let the lookup mask the failed command, count it, or drop
// the qualifier, and this reads nothing, "2 recorded failures" or a bare "1
// recorded failure.".
func TestH91_AFailedLookupBesideARealFailureIsNotCounted(t *testing.T) {
	for _, tc := range []struct {
		lookup bool
		want   string
	}{
		{true, "rashomon: 1 recorded failure, besides 1 failed lookup.\n"},
		{false, "rashomon: 1 recorded failure.\n"},
	} {
		e := newEnv(t)
		e.watched(testSession)
		if tc.lookup {
			readOfADirectory(t, e, "toolu_read")
		}
		p := defaultPayload()
		p.ToolUseID = "toolu_make"
		e.mustHook(p.build(t))
		e.mustPost(failurePayload(t, "toolu_make", "Exit code 2", false, 30))

		line, ok := e.recapLine(stopPayload(testSession, "All 9 tests pass.", false))
		if !ok || !strings.Contains(line, tc.want) {
			t.Errorf("line = %q (printed %v), want %q", line, ok, tc.want)
		}
	}
}
