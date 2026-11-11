package acceptance

// H-171: a project whose Makefile has a test target, and the turn's only
// call is a sed -i over two files that failed with exit 2 after rewriting
// the first. The base printed "2 files edited, make test (inferred) not run
// after the last edit." and owed make test at the next prompt; the reviewed
// base fix printed nothing at either, as it does for a failed Edit alone
// (H-161), so the person saw no sign a file may have changed untested. The
// Stop now names the call and that make test was not run after it. The
// next prompt still carries no owed-gate note: that note says "after the
// last edit", an edit the record does not hold, which is the rule's stated
// cost (classify). Its twin, the sed ok, gets the line and the note, so the
// absence of the note above is the failed call's and not a note never
// given. The risk it holds down is a turn that may have written going
// silent because nothing ended ok.

import "testing"

const makefileStart = `session start:
  systemMessage:
    | rashomon brief: {project}
    |   instructions: none (CLAUDE.md, .claude/CLAUDE.md, AGENTS.md, user CLAUDE.md)
    |   build: Makefile
    |   tests: "make test" (inferred from Makefile)
    |   spec: none (.claude/rashomon/spec.json)
    |   task: none (task/current.json)
    |   packages: not read for this language (go.mod only)
    |   verify: rashomon report --session sess-1
`

func TestH171_AFailedShellWriteAloneIsNamed(t *testing.T) {
	holdLine(t, "lines/failed-shell-write-alone-makefile", lineRun(t, "failed-shell-write-alone-makefile"),
		`=== lines/failed-shell-write-alone-makefile turn 1 of 1
prompt: prompt-1
digest read: prompt-1
fired: none
`+makefileStart+`calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: call 1 failed and may have written files; make test not run after it.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
}

// TestH171_TwinTheSedOK: the same call ended ok edited both files and owes
// make test, at the Stop and at the next prompt.
func TestH171_TwinTheSedOK(t *testing.T) {
	f := lineRun(t, "failed-shell-write-alone-makefile")
	f.Calls[0].Outcome, f.Calls[0].ExitCode, f.Calls[0].Output = "ok", 0, ""
	holdLine(t, "lines/failed-shell-write-alone-makefile, the sed ok", f,
		`=== lines/failed-shell-write-alone-makefile, the sed ok turn 1 of 1
prompt: prompt-1
digest read: prompt-1
fired: none
`+makefileStart+`calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: 2 files edited, make test (inferred) not run after the last edit.
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
