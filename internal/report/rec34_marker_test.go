package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestREC34i_AMarkerNamingNoCallIsNoLostCall: a flush_failed marker says the
// process's records may not be durable; it is evidence of a call only when it
// names one. A marker with no tool_use_id comes from a hook or post that never
// learned which call it was, which with a successful flush leaves no terminal
// at all and counts as no lost call, so the failed flush must not add one and
// with it suppress every test-bending pair in the session. A marker that names
// a call no declaration covers still counts: that call ran, and may have been
// an edit (REC-34, P09-F7).
func TestREC34i_AMarkerNamingNoCallIsNoLostCall(t *testing.T) {
	reason := store.ReasonFlushFailed
	marker := func(id string) store.Terminal {
		return store.Terminal{Type: store.TypeTerminal, ToolUseID: id, Outcome: store.OutcomeError, Reason: &reason}
	}
	run := &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "toolu_a"}},
		Terminals:    []store.Terminal{{ToolUseID: "toolu_a", Outcome: store.OutcomeOK}, marker("")},
	}
	if got := undeclaredCalls(run, nil); got != 0 {
		t.Errorf("undeclaredCalls with an empty-id marker = %d, want 0: a marker naming no call read as a lost one", got)
	}
	run.Terminals = append(run.Terminals, marker("toolu_b"))
	if got := undeclaredCalls(run, nil); got != 1 {
		t.Errorf("undeclaredCalls with a marker for an undeclared call = %d, want 1", got)
	}
}
