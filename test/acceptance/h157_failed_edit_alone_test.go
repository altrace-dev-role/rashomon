package acceptance

// H-157: a turn whose only call is an Edit that failed edited nothing, and
// its Stop says nothing about edits (T1). The base printed "1 file edited; no
// test or build is known for this project yet." for a project with no test
// command: classify appended a write for every declaration whatever its
// ending, and the turn's counts read it. An absence renders as an absence:
// with no edit and no gate run there is no line. The risk it holds down is a
// reader of the turn's counts that counts a call the tool reported it did not
// apply. Its twin is the same run with the Edit applied, whose line counts it,
// so the silence is not a Stop that prints nothing at all.

import "testing"

func TestH157_AFailedEditAloneIsNoEdit(t *testing.T) {
	holdLine(t, "lines/t1-failed-edit-only-call", lineRun(t, "t1-failed-edit-only-call"),
		`=== lines/t1-failed-edit-only-call turn 1 of 1
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
calls: 1; call events that printed: 0
stop:
  nothing
next prompt prompt-next:
  nothing
`)
}

// TestH157_TwinTheEditApplied: the same run with the Edit ok.
func TestH157_TwinTheEditApplied(t *testing.T) {
	f := lineRun(t, "t1-failed-edit-only-call")
	f.Calls[0].Outcome, f.Calls[0].Output = "ok", ""
	holdLine(t, "lines/t1-failed-edit-only-call, the Edit ok", f,
		`=== lines/t1-failed-edit-only-call, the Edit ok turn 1 of 1
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
calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: 1 file edited; no test or build is known for this project yet.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}
