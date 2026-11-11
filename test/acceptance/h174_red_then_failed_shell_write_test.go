package acceptance

// H-174: Edit, go test fails with exit 1, then a sed -i naming internal/a.go
// fails with exit 2 after it may have rewritten a.go. The reviewed base fix
// printed "1 file edited; go test exited 1 after the last edit (call 4).":
// true of the edits the record holds, and silent on a call that may have
// changed the code after the run, so the reader took the red result as
// describing the code as it stands, which was not measured. The base before
// the fix read the sed as the last edit and said go test had not run since.
// The line names the call beside the run (TurnCounts.WroteAfter). The twin
// replays the same run with an Edit that failed in the sed's place: it
// applied nothing, the run saw every change, and the line is the red row
// alone. The risk it holds down is a run placed after the last edit while a
// call that may have written stands after it unnamed.

import "testing"

func TestH174_ARedRunNamesACallThatMayHaveWrittenAfterIt(t *testing.T) {
	const head = `prompt: prompt-1
digest read: prompt-1
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
calls: 3; call events that printed: 0
stop:
  systemMessage:
`
	holdLine(t, "lines/red-then-failed-shell-write", lineRun(t, "red-then-failed-shell-write"),
		"=== lines/red-then-failed-shell-write turn 1 of 1\n"+head+
			`    | ※ rashomon: 1 file edited; go test exited 1 after the last edit (call 4); call 7 failed and may have written files after that.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)

	twin := lineRun(t, "red-then-failed-shell-write")
	twin.Calls[2] = fixtureCall{Tool: "Edit", Outcome: "failed", Output: "String to replace not found in file.",
		Input: map[string]any{"file_path": "{project}/internal/a.go", "old_string": "c", "new_string": "d"}}
	holdLine(t, "lines/red-then-failed-edit", twin,
		"=== lines/red-then-failed-edit turn 1 of 1\n"+head+
			`    | ※ rashomon: 1 file edited; go test exited 1 after the last edit (call 4).
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
