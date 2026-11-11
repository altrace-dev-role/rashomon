package acceptance

// H-172: after a green go test, a sed -i fails after it may have rewritten
// internal/a.go, then an Edit of internal/c.go fails and applies nothing.
// The reviewed base fix kept only the later held call and named the Edit:
// "call 10, an edit, failed after that", the sentence for a call that
// changed nothing, so a reader took the green as standing while the call
// that may have rewritten a.go went unnamed. The line names the call that
// may have written before a failed edit (TurnCounts.unconfirmed). The risk
// it holds down is the harmless call hiding the one the row exists to name.

import "testing"

func TestH172_AFailedEditDoesNotHideACallThatMayHaveWritten(t *testing.T) {
	holdLine(t, "lines/failed-shell-write-then-failed-edit-after-green", lineRun(t, "failed-shell-write-then-failed-edit-after-green"),
		`=== lines/failed-shell-write-then-failed-edit-after-green turn 1 of 1
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
calls: 4; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: 1 file edited; go test passed (call 4); call 7 failed and may have written files after that.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
