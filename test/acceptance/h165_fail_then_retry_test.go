package acceptance

// H-165: the common retry: an Edit fails, the retried Edit applies, go test
// passes after it. The failed Edit came before the green run the line rests
// on, so the line reads verified, as the control does: the guard that names
// a failed call reads only calls declared after that green. The risk it
// holds down is a guard that qualifies every turn holding a failed edit,
// which would withhold verified from the commonest turn there is.

import "testing"

func TestH165_AFailedEditBeforeTheGreenLeavesVerified(t *testing.T) {
	holdLine(t, "lines/fail-then-retry-green-test-done", lineRun(t, "fail-then-retry-green-test-done"),
		`=== lines/fail-then-retry-green-test-done turn 1 of 1
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
    | ※ rashomon: verified - go test (inferred) passed after the last edit.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
