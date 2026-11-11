package acceptance

// H-161: a project whose Makefile has a test target, and the turn's only
// Edit failed (T9a). The base printed "1 file edited, make test (inferred)
// not run after the last edit." and told the model at the next prompt
// "gate 'make test' (from 'Makefile') was not run after the last edit."
// Both stated an edit that did not happen; now neither prints. The owed-gate
// note follows from the same counts as the line (owedGate reads Turn.Writes
// and the gate's writes since green). The positive twin is the same project
// with the Edit applied (t9a-twin-edit-makefile): the line and the note are
// both given, so their absence above is not a note that is never given.

import "testing"

func TestH161_NoGateIsOwedAfterAFailedEdit(t *testing.T) {
	holdLine(t, "lines/t9a-failed-edit-makefile", lineRun(t, "t9a-failed-edit-makefile"),
		`=== lines/t9a-failed-edit-makefile turn 1 of 1
prompt: prompt-1
digest read: prompt-1
fired: none
session start:
  systemMessage:
    | rashomon brief: {project}
    |   instructions: none (CLAUDE.md, .claude/CLAUDE.md, AGENTS.md, user CLAUDE.md)
    |   build: Makefile
    |   tests: "make test" (inferred from Makefile)
    |   spec: none (.claude/rashomon/spec.json)
    |   task: none (task/current.json)
    |   packages: not read for this language (go.mod only)
    |   verify: rashomon report --session sess-1
calls: 1; call events that printed: 0
stop:
  nothing
next prompt prompt-next:
  nothing
`)
}

// TestH161_TwinTheEditApplied: the same project with the Edit applied owes
// the gate: the line and the next prompt's note are both given.
func TestH161_TwinTheEditApplied(t *testing.T) {
	holdLine(t, "lines/t9a-twin-edit-makefile", lineRun(t, "t9a-twin-edit-makefile"),
		`=== lines/t9a-twin-edit-makefile turn 1 of 1
prompt: prompt-1
digest read: prompt-1
fired: none
session start:
  systemMessage:
    | rashomon brief: {project}
    |   instructions: none (CLAUDE.md, .claude/CLAUDE.md, AGENTS.md, user CLAUDE.md)
    |   build: Makefile
    |   tests: "make test" (inferred from Makefile)
    |   spec: none (.claude/rashomon/spec.json)
    |   task: none (task/current.json)
    |   packages: not read for this language (go.mod only)
    |   verify: rashomon report --session sess-1
calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: 1 file edited, make test (inferred) not run after the last edit.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  hookSpecificOutput.additionalContext:
    | rashomon record: gate 'make test' (from 'Makefile') was not run after the last edit.
  hookSpecificOutput.hookEventName:
    | UserPromptSubmit
  systemMessage:
    | ※ rashomon told the agent: "rashomon record: gate 'make test' (from 'Makefile') was not run after the last edit."
`)
}
