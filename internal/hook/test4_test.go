package hook

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRubricTEST4_ALockTimeoutLeavesASpilledTerminal: a declaration that
// could not take the ordered stream's lock is not on disk, and the terminal
// Close writes to the spill file is then the only trace of its tool_use_id.
// No test took this branch, and deleting the SpillTerminal call passed the
// suite: the call would then read afterwards as one that was never asked
// for. The terminal carries outcome error, reason lock_timeout and no seq,
// and the call's coverage says lock_timeout.
func TestRubricTEST4_ALockTimeoutLeavesASpilledTerminal(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	h := New(st, time.Now)
	// What Capture leaves behind when AppendDeclaration times out: the call
	// is attributed and named, and nothing was opened.
	h.sessionID, h.toolUseID = "sess-lock", "toolu_lock"
	h.Close(&Signals{}, store.ErrLockTimeout)

	run, err := st.ReadRun("sess-lock")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 0 {
		t.Fatalf("premise broken: %d declarations on disk", len(run.Declarations))
	}
	if len(run.Terminals) != 1 {
		t.Fatalf("got %d terminals, want the one spilled terminal naming the dropped call", len(run.Terminals))
	}
	term := run.Terminals[0]
	if term.ToolUseID != "toolu_lock" || term.Outcome != store.OutcomeError || term.Seq != nil ||
		term.Reason == nil || *term.Reason != store.ReasonLockTimeout {
		t.Errorf("terminal %+v, want toolu_lock, outcome error, reason lock_timeout and a null seq", term)
	}
	if got := run.Dropped(); len(got) != 1 || got[0] != "toolu_lock" {
		t.Errorf("Dropped() = %v, want the call whose declaration could not land", got)
	}
	if len(run.Coverage) != 1 || run.Coverage[0].Reason == nil || *run.Coverage[0].Reason != store.ReasonLockTimeout {
		t.Errorf("coverage %+v, want one record carrying lock_timeout", run.Coverage)
	}
}

// TestRubricTEST4_AnUnnamedCallGetsNoTerminal is the branch's other side: a
// lock timeout on a call with no tool_use_id has nothing to name, so no
// terminal is written, and the coverage still says lock_timeout.
func TestRubricTEST4_AnUnnamedCallGetsNoTerminal(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	h := New(st, time.Now)
	h.sessionID = "sess-lock"
	h.Close(&Signals{}, store.ErrLockTimeout)

	run, err := st.ReadRun("sess-lock")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Terminals) != 0 {
		t.Errorf("got %d terminals for a call with no tool_use_id", len(run.Terminals))
	}
	if len(run.Coverage) != 1 || run.Coverage[0].Reason == nil || *run.Coverage[0].Reason != store.ReasonLockTimeout {
		t.Errorf("coverage %+v, want one record carrying lock_timeout", run.Coverage)
	}
}
