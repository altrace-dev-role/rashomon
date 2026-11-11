package acceptance

// H-163: Edit, go test passes, then a second Edit that was refused and has
// no ending. The Stop prints the detector's sentence; the state line under it
// names the call with no recorded ending after the green run, which the
// report reads back, and the next prompt carries nothing. The base's state
// line was "1 file edited, go test (inferred) not run after the last edit.",
// and its next prompt told the model the previous turn's claim was not
// backed by "no passing test or build run after its last edit", an edit the
// record does not hold. The cost of the rule shows here: an Edit that did
// apply and lost its post would get the same silence at the next prompt; the
// state line still names it.

import "testing"

func TestH163_AnUnendedEditAfterTheGreen(t *testing.T) {
	e := holdLine(t, "lines/unended-edit-after-green", lineRun(t, "unended-edit-after-green"),
		`=== lines/unended-edit-after-green turn 1 of 1
prompt: prompt-1
digest read: prompt-1
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
next prompt prompt-next:
  nothing
`)
	const stop = "last Stop: 1 file edited; go test passed (call 4); call 7 has no recorded ending (call 8)"
	if got := e.lastStopLine(); got != stop {
		t.Errorf("report: %q, want %q", got, stop)
	}
}
