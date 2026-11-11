package acceptance

// H-164: the first turn edits, passes go test and starts a second Edit that
// never ends; the second turn makes no call and says Done. The base told the
// model at the first turn's next prompt that its claim was not backed, and
// at the second's that "gate 'go test' (from 'go.mod') was not run after the
// last edit": the unended Edit read as that edit. go test passed after the
// last edit the record holds, so neither note is given. The second turn's
// Stop is judged on the first turn's prompt (digest read), as the base's
// was. Its twin is the same run with the second Edit ended ok: both notes
// are given, so their absence here is the unended Edit's and not notes the
// replay never computes.

import "testing"

func TestH164_AnUnendedEditOwesNoGateAtTheNextTurn(t *testing.T) {
	holdLine(t, "lines/two-turn-unended-edit-then-done", lineRun(t, "two-turn-unended-edit-then-done"),
		`=== lines/two-turn-unended-edit-then-done turn 1 of 2
prompt: 00000000-0000-4000-8000-000000000101
digest read: 00000000-0000-4000-8000-000000000101
fired: declaration_without_execution
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
    | ※ rashomon: 1 declaration without recorded execution.
    |             → rashomon report --session sess-1
next prompt 00000000-0000-4000-8000-000000000102:
  nothing
`,
		`=== lines/two-turn-unended-edit-then-done turn 2 of 2
prompt: 00000000-0000-4000-8000-000000000102
digest read: 00000000-0000-4000-8000-000000000101
fired: declaration_without_execution
calls: 0; call events that printed: 0
stop:
  nothing
next prompt prompt-next:
  nothing
`)
}

// TestH164_TwinTheEditApplied: the first turn's second Edit ended ok. It is
// the last edit, go test was not run after it, and the claim of done the
// second turn read is not backed: the done-unverified note follows the
// first turn and the owed-gate note the second.
func TestH164_TwinTheEditApplied(t *testing.T) {
	f := lineRun(t, "two-turn-unended-edit-then-done")
	f.Turns[0].Calls[2].Outcome = "ok"
	const doneNote = "rashomon record: the previous turn ended with a claim of completion that the record does not back " +
		"(no passing test or build run after its last edit, or expected evidence missing)."
	const owedNote = "rashomon record: gate 'go test' (from 'go.mod') was not run after the last edit."
	holdLine(t, "lines/two-turn-unended-edit-then-done, the Edit ok", f,
		`=== lines/two-turn-unended-edit-then-done, the Edit ok turn 1 of 2
prompt: 00000000-0000-4000-8000-000000000101
digest read: 00000000-0000-4000-8000-000000000101
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
    | ※ rashomon: done was claimed; the record shows 1 file edited, go test not run after the last edit.
    |             → rashomon report --session sess-1
next prompt 00000000-0000-4000-8000-000000000102:
  hookSpecificOutput.additionalContext:
    | `+doneNote+`
  hookSpecificOutput.hookEventName:
    | UserPromptSubmit
  systemMessage:
    | ※ rashomon told the agent: "`+doneNote+`"
`,
		`=== lines/two-turn-unended-edit-then-done, the Edit ok turn 2 of 2
prompt: 00000000-0000-4000-8000-000000000102
digest read: 00000000-0000-4000-8000-000000000101
fired: none
calls: 0; call events that printed: 0
stop:
  nothing
next prompt prompt-next:
  hookSpecificOutput.additionalContext:
    | `+owedNote+`
  hookSpecificOutput.hookEventName:
    | UserPromptSubmit
  systemMessage:
    | ※ rashomon told the agent: "`+owedNote+`"
`)
}
