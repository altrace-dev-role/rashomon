package acceptance

// H-160: a go.mod project whose turn's only call is a Write that failed
// (T6). Nothing was written, so no file was edited and no test is owed
// after an edit: the Stop is silent. The base printed "1 file edited, go
// test (inferred) not run after the last edit." Its twin, the Write applied,
// gets that line, so the silence is the failed Write's.

import "testing"

func TestH160_AFailedWriteAloneIsNoEdit(t *testing.T) {
	holdLine(t, "lines/t6-failed-write-only-call", lineRun(t, "t6-failed-write-only-call"),
		`=== lines/t6-failed-write-only-call turn 1 of 1
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
calls: 1; call events that printed: 0
stop:
  nothing
next prompt prompt-next:
  nothing
`)
}

// TestH160_TwinTheWriteApplied: the same run with the Write ok.
func TestH160_TwinTheWriteApplied(t *testing.T) {
	f := lineRun(t, "t6-failed-write-only-call")
	f.Calls[0].Outcome, f.Calls[0].Output = "ok", ""
	holdLine(t, "lines/t6-failed-write-only-call, the Write ok", f,
		`=== lines/t6-failed-write-only-call, the Write ok turn 1 of 1
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
calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: 1 file edited, go test (inferred) not run after the last edit.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
