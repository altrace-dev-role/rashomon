package acceptance

// H-173: after a green go test, the person interrupts an Edit. Claude Code
// reports the interrupt on the failure event, and the reviewed base fix
// read it as a failure: "call 7 failed and may have written files after
// that". Nothing failed; the record holds an interrupt, and the line says
// so, and still that the call may have written, so it never reads
// verified. The risk it holds down is the line stating an ending the record
// does not hold.

import "testing"

func TestH173_AnInterruptedCallIsNamedAsInterrupted(t *testing.T) {
	holdLine(t, "lines/interrupted-edit-after-green", lineRun(t, "interrupted-edit-after-green"),
		`=== lines/interrupted-edit-after-green turn 1 of 1
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
    | ※ rashomon: 1 file edited; go test passed (call 4); call 7 was interrupted and may have written files after that.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
