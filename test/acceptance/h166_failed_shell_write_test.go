package acceptance

// H-166: Edit, go test passes, then a sed -i naming internal/a.go fails with
// exit 2. Whether sed wrote before it failed is not in the record (here it
// did: GNU sed rewrites the first file before it fails on the second). The
// line says what was measured and that the call failed and may have written
// files after the green; it never reads verified, and never says go test was
// not run after the last edit, an edit the record does not hold. The risk it
// holds down is a failed shell write read as an edit (stale gate) or as no
// write (verified).

import "testing"

func TestH166_AFailedShellWriteAfterTheGreenMayHaveWritten(t *testing.T) {
	holdLine(t, "lines/failed-shell-write-after-green", lineRun(t, "failed-shell-write-after-green"),
		`=== lines/failed-shell-write-after-green turn 1 of 1
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
    | ※ rashomon: 1 file edited; go test passed (call 4); call 7 failed and may have written files after that.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
