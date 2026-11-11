package acceptance

// H-159: T2 in a project with no manifest, where go test is a learned gate
// and not an inferred one. The base printed "1 file edited, no test or build
// run after the last edit.", false twice over: go test ran after the only
// edit that applied. The line names the green run and the failed Edit after
// it, as H-158 does for the inferred gate: one rule, whichever way the gate
// came to be known.

import "testing"

func TestH159_AFailedEditAfterTheGreenWithNoManifest(t *testing.T) {
	holdLine(t, "lines/t2-failed-edit-after-green-no-manifest", lineRun(t, "t2-failed-edit-after-green-no-manifest"),
		`=== lines/t2-failed-edit-after-green-no-manifest turn 1 of 1
prompt: prompt-1
digest read: prompt-1
fired: none
session start:
  systemMessage:
    | rashomon brief: {project}
    |   instructions: none (CLAUDE.md, .claude/CLAUDE.md, AGENTS.md, user CLAUDE.md)
    |   build: no manifest at the root (go.mod, package.json, ...)
    |   tests: no spec gate; no test runner in the index yet
    |   spec: none (.claude/rashomon/spec.json)
    |   task: none (task/current.json)
    |   packages: not read (no go.mod)
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
