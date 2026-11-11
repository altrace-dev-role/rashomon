package acceptance

// H-168: two gates pass in one turn, go test (call 4) before an Edit that
// failed (call 7) and npm test (call 10) after it. The guard reads the
// failed Edit against the earliest last green, go test's, so the line names
// it; the line's own "(call 10)" is the latest green's, and a clause "after
// that" placed call 7 after a run it came before. The line places the call
// by the green the guard read it against, by name and call. The risk it
// holds down is a clause placed by one green and judged by another: the
// review's replay printed "go test and 1 more passed (call 10); call 7, an
// edit, failed after that." The unnamed_write row shares the placement
// (TestWEND9_).

import "testing"

func TestH168_ACallBetweenTwoGreensIsPlacedByTheRunItFollowed(t *testing.T) {
	e := holdLine(t, "lines/two-gates-failed-edit-between-greens", lineRun(t, "two-gates-failed-edit-between-greens"),
		`=== lines/two-gates-failed-edit-between-greens turn 1 of 1
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
    | ※ rashomon: 1 file edited; go test and 1 more passed (call 10); call 7, an edit, failed after go test passed (call 4).
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
	const stop = "last Stop: 1 file edited; go test and 1 more passed (call 10); call 7, an edit, failed after go test passed (call 4) (call 12)"
	if got := e.lastStopLine(); got != stop {
		t.Errorf("report: %q, want %q", got, stop)
	}
}
