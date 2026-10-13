package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
)

// spillAt spills a lock_timeout terminal for id, recorded at ms.
func spillAt(t *testing.T, st *Store, session, id string, ms int64) {
	t.Helper()
	reason := ReasonLockTimeout
	if err := st.SpillTerminal(Terminal{Type: TypeTerminal, SchemaVersion: SchemaVersion, RecordedAtMS: ms,
		ToolUseID: id, SessionID: session, Outcome: OutcomeError, Reason: &reason}); err != nil {
		t.Fatal(err)
	}
}

// declareAt appends a declaration for id, recorded at ms, naming hosts.
func declareAt(t *testing.T, st *Store, session, id string, ms int64, hosts ...string) {
	t.Helper()
	d := testDecl(session, id)
	d.RecordedAtMS, d.Hosts = ms, hosts
	if err := st.AppendDeclaration(d); err != nil {
		t.Fatal(err)
	}
}

// spilledIDs is the tool_use_ids of the terminals in a session's spill file.
func spilledIDs(t *testing.T, st *Store, session string) map[string]bool {
	t.Helper()
	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, term := range run.Terminals {
		if term.Seq == nil {
			ids[term.ToolUseID] = true
		}
	}
	return ids
}

// TestRubricTEST2_ForgetWindowKeepsSpillOutsideTheWindow: a forget that
// removes one spilled terminal writes every other spill line back. The spill
// rewrite ran at 0% coverage, and a rewrite that wiped every kept spill line
// passed the suite, with a gap that counted only what the plan removed.
func TestRubricTEST2_ForgetWindowKeepsSpillOutsideTheWindow(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-window"
	declareAt(t, st, session, "toolu_in", 1000)
	spillAt(t, st, session, "toolu_in", 1000)
	spillAt(t, st, session, "toolu_out", 5000)

	cut := time.UnixMilli(3000)
	gaps, err := st.ForgetWindow(nil, &cut, time.UnixMilli(9000))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].RemovedRecords != 2 {
		t.Fatalf("gaps %+v, want one gap counting the declaration and its spilled terminal", gaps)
	}
	if got := spilledIDs(t, st, session); len(got) != 1 || !got["toolu_out"] {
		t.Errorf("spill file holds %v after forget --before, want only toolu_out", got)
	}
}

// TestRubricTEST2_ForgetHostKeepsSpillOfOtherCalls is the window test's twin
// for forget --host, which rewrites the spill file at a site of its own.
func TestRubricTEST2_ForgetHostKeepsSpillOfOtherCalls(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-host"
	declareAt(t, st, session, "toolu_a", 1000, "a.example")
	declareAt(t, st, session, "toolu_b", 2000, "b.example")
	spillAt(t, st, session, "toolu_a", 1000)
	spillAt(t, st, session, "toolu_b", 2000)

	gaps, err := st.ForgetHost("a.example", time.UnixMilli(9000))
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].RemovedRecords != 2 {
		t.Fatalf("gaps %+v, want one gap counting toolu_a's declaration and spilled terminal", gaps)
	}
	if got := spilledIDs(t, st, session); len(got) != 1 || !got["toolu_b"] {
		t.Errorf("spill file holds %v after forget --host, want only toolu_b", got)
	}
}

// TestRubricCONC1_ForgetKeepsASpillAppendedAfterItsPlan: forget plans from a
// read of the spill file taken without the spill lock, and writes the gap
// before it rewrites. A spill that lands between the two -- here, while the
// test holds the gaps lock forget is waiting for -- was erased by a rewrite
// from the stale plan, with no gap record. Spills include lock_timeout
// terminals, the only trace of a declaration that could not land.
func TestRubricCONC1_ForgetKeepsASpillAppendedAfterItsPlan(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-race"
	declareAt(t, st, session, "toolu_in", 1000)
	spillAt(t, st, session, "toolu_in", 1000)

	gf, err := os.OpenFile(filepath.Join(st.Root(), FileGaps), appendFlags, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer gf.Close()
	unlock, err := lockFile(gf, lockBudget)
	if err != nil {
		t.Fatal(err)
	}

	cut := time.UnixMilli(3000)
	var forgetErr error
	done := safe.Go(func() {
		_, forgetErr = st.ForgetWindow(nil, &cut, time.UnixMilli(9000))
	}, func(err error) { forgetErr = err })
	// The plan is microseconds of reading; forget is then waiting on the
	// gaps lock, with up to lockBudget to wait.
	time.Sleep(200 * time.Millisecond)
	spillAt(t, st, session, "toolu_late", 6000)
	unlock()
	<-done
	if forgetErr != nil {
		t.Fatal(forgetErr)
	}

	got := spilledIDs(t, st, session)
	if !got["toolu_late"] {
		t.Errorf("the terminal spilled after forget's plan was erased: spill holds %v", got)
	}
	if got["toolu_in"] {
		t.Errorf("forget left the spilled terminal it counted: spill holds %v", got)
	}
}

// TestRubricCONC1_SpillRewriteDropsOnlyCountedLines: the rewrite under the
// spill lock removes each counted line once, as a multiset, and writes back
// every line it did not count, a duplicate of a counted one included.
func TestRubricCONC1_SpillRewriteDropsOnlyCountedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileSpill)
	body := "a\nb\na\nc\n"
	if err := os.WriteFile(path, []byte(body), fileMode); err != nil {
		t.Fatal(err)
	}
	snapshot := [][]byte{[]byte("a"), []byte("b"), []byte("a")}
	kept := [][]byte{[]byte("b"), []byte("a")}
	if err := rewriteSpill(path, snapshot, kept); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "b\na\nc\n" {
		t.Errorf("spill after the rewrite is %q, want %q", got, "b\na\nc\n")
	}
	if err := rewriteSpill(filepath.Join(t.TempDir(), FileSpill), snapshot, kept); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rewriting a spill file that is gone: %v, want not-exist", err)
	}
}
