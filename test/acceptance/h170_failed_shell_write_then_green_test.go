package acceptance

// H-170: a sed -i over two files fails after rewriting the first, and go
// test passes after it; no call ended ok as an edit. The base printed
// "verified - go test (inferred) passed after the last edit.", which was
// true, since it counted the sed. The reviewed base fix printed "no edits;
// go test (inferred) passed (call 4).": the sed may have edited a.go, and
// "no edits" speaks for the whole turn, so it claimed what was not
// measured. The line now names the call and places it before the green run
// that came after whatever it wrote. The risk it holds down is the ran_only
// row read past a call that may have written; H-165 holds the fail-then-
// retry turn, a failed Edit before the green, to verified.

import "testing"

func TestH170_ACallThatMayHaveWrittenBeforeTheGreenIsNotNoEdits(t *testing.T) {
	holdLine(t, "lines/failed-shell-write-then-green", lineRun(t, "failed-shell-write-then-green"),
		`=== lines/failed-shell-write-then-green turn 1 of 1
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
calls: 2; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: go test passed (call 4); call 1 failed and may have written files before that.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
