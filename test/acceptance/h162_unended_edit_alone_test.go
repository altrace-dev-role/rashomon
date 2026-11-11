package acceptance

// H-162: a go.mod project whose turn's only call is an Edit with no
// recorded ending, and no post ever ran, so the record's coverage is not
// verified: the record is incomplete. Whether the Edit wrote is not in the
// record, and it is no edit; but an incomplete turn that may have written
// reads unknown, never none (D80), so the state line the Stop computed,
// which the report reads back, stays "recording incomplete for this turn;
// verification state unknown", as on the base. The Stop prints the
// detector's sentence, which counts the declaration, and nothing else. The
// risk it holds down is the rule's own over-reach: an unended write dropped
// from the counts that also drops the turn to silence where the record has
// a gap. This item passes on the base; it fails when the incomplete record's
// unknown row stops reading held writes.

import "testing"

func TestH162_AnUnendedEditInAnIncompleteRecordReadsUnknown(t *testing.T) {
	e := holdLine(t, "lines/unended-edit-only-call", lineRun(t, "unended-edit-only-call"),
		`=== lines/unended-edit-only-call turn 1 of 1
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
calls: 1; call events that printed: 0
stop:
  systemMessage:
    | ※ rashomon: 1 declaration without recorded execution.
    |             → rashomon report --session sess-1
next prompt prompt-next:
  nothing
`)
	const stop = "last Stop: recording incomplete for this turn; verification state unknown (call 2)"
	if got := e.lastStopLine(); got != stop {
		t.Errorf("report: %q, want %q", got, stop)
	}
}
