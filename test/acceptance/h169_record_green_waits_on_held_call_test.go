package acceptance

// H-169: the first turn edits and passes go test; the second turn's only
// call is a sed -i that failed after it may have rewritten a file go test
// checks. No gate is stale, since the record does not say sed wrote, but the
// record does not back a "done" either: the second turn's Stop names the
// call and that go test was not run after it, and the next session's start
// line reads "not green" where the reviewed base fix printed "record green"
// and turn 2's Stop printed nothing. The base, which counted the sed as an
// edit, printed "1 gate not green". The positive twin is the same run with
// the second turn's call a failed Edit, which applied nothing: its Stop is
// silent and the start line reads "record green", so the "not green" above
// is the sed's and not a line that never says green. The risk it holds down
// is a record green point read from the gates alone while a call that may
// have written stands after the green.

import (
	"strings"
	"testing"
)

// turn1Green is the first turn's block, the same in both runs.
const turn1Green = `=== lines/two-turn-failed-shell-write-after-green turn 1 of 2
prompt: 00000000-0000-4000-8000-000000000201
digest read: 00000000-0000-4000-8000-000000000201
fired: none
session start:
  systemMessage:
    | rashomon brief: {project}
    |   instructions: none (CLAUDE.md, .claude/CLAUDE.md, AGENTS.md, user CLAUDE.md)
    |   build: go.mod (module m)
    |   tests: "go test ./..." (inferred from go.mod)
    |   spec: none (.claude/rashomon/spec.json)
    |   task: none (task/current.json)
    |   packages: 0 Go directories (walk)
    |   verify: rashomon report --session sess-1
calls: 2; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: verified - go test (inferred) passed after the last edit.
    |             → rashomon report --session sess-1
next prompt 00000000-0000-4000-8000-000000000202:
  nothing
`

// nextSessionLine is the "last session:" line of a new session's start in
// the same project, its time left out.
func nextSessionLine(t *testing.T, e *env) string {
	t.Helper()
	res := e.probe("start", "sess-2", replayEnv...)
	if res.exitCode != 0 {
		t.Fatalf("start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	_, line, ok := strings.Cut(res.stdout, "last session: ")
	if !ok {
		t.Fatalf("the start names no last session:\n%s", res.stdout)
	}
	_, reason, ok := strings.Cut(line, ", ")
	if !ok {
		t.Fatalf("last session line %q has no reason", line)
	}
	reason, _, _ = strings.Cut(reason, ";")
	return reason
}

func TestH169_ACallThatMayHaveWrittenAfterTheGreenLeavesTheRecordNotGreen(t *testing.T) {
	e := holdLine(t, "lines/two-turn-failed-shell-write-after-green", lineRun(t, "two-turn-failed-shell-write-after-green"),
		turn1Green,
		`=== lines/two-turn-failed-shell-write-after-green turn 2 of 2
prompt: 00000000-0000-4000-8000-000000000202
digest read: 00000000-0000-4000-8000-000000000202
fired: none
calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: call 8 failed and may have written files; go test not run after it.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
	if got := nextSessionLine(t, e); got != "not green" {
		t.Errorf("the next session's start line reads %q, want \"not green\"", got)
	}
}

// TestH169_TwinAFailedEditInstead: the second turn's call a failed Edit.
func TestH169_TwinAFailedEditInstead(t *testing.T) {
	f := lineRun(t, "two-turn-failed-shell-write-after-green")
	f.Turns[1].Calls[0] = fixtureCall{Tool: "Edit", Input: map[string]any{"file_path": "{project}/internal/a.go", "old_string": "x", "new_string": "y"},
		Outcome: "failed", Output: "String to replace not found in file."}
	e := holdLine(t, "lines/two-turn-failed-shell-write-after-green", f, turn1Green,
		`=== lines/two-turn-failed-shell-write-after-green turn 2 of 2
prompt: 00000000-0000-4000-8000-000000000202
digest read: 00000000-0000-4000-8000-000000000202
fired: none
calls: 1; call events that printed: 0
stop:
  nothing
next prompt prompt-next:
  nothing
`)
	if got := nextSessionLine(t, e); got != "record green" {
		t.Errorf("the next session's start line reads %q, want \"record green\"", got)
	}
}
