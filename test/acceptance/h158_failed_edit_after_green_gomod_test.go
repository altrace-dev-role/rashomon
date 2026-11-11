package acceptance

// H-158: Edit, go test passes, then a second Edit of the same file fails (T2,
// in a go.mod project). The last edit is the first one, and go test passed
// after it; the line says so, and names the failed Edit after the run in
// closed words, never "verified" while a call after the green did not end ok.
// The base printed "1 file edited, go test (inferred) not run after the last
// edit.", the failed Edit read as the last edit and the gate stale. The risk
// it holds down is a gate made stale by a write that did not happen, and the
// row's guard dropped (TurnSummary reading verified past the failed call).

import "testing"

func TestH158_AFailedEditAfterTheGreenIsNamed(t *testing.T) {
	holdLine(t, "lines/t2-failed-edit-after-green-go-mod", lineRun(t, "t2-failed-edit-after-green-go-mod"),
		`=== lines/t2-failed-edit-after-green-go-mod turn 1 of 1
prompt: prompt-1
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
    | ※ rashomon: 1 file edited; go test passed (call 4); call 7, an edit, failed after that.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
