//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRubricPERF1_TheBudgetCoversTheWholeRead: ReadRunConsistent's budget is
// for the whole read, not for each of its three files. Per file, three held
// files cost three budgets plus three polling overshoots: digest's 10 ms
// gave up after about 38 ms, three-quarters of the 50 ms the command owes,
// which is why the budget could not grow to wait out a longer hold. With one
// deadline across the read, a single held file may take all of it and three
// held files cost no more.
//
// Unix-only for the reason TestReadRunConsistent_RespectsAHeldLock is: the
// read past a held lock it falls back to is refused on Windows.
func TestRubricPERF1_TheBudgetCoversTheWholeRead(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const session = "sess-three-held"
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"declaration","schema_version":2,"seq":1,"tool_use_id":"toolu_1","session_id":"sess-three-held"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, FileRecords), []byte(line), fileMode); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{FileRecords, FileSpill, FileCoverage} {
		unlock, _, err := lockFile(openForAppend(t, filepath.Join(dir, name)), lockBudget)
		if err != nil {
			t.Fatalf("taking the %s lock: %v", name, err)
		}
		defer unlock()
	}

	const budget = 30 * time.Millisecond
	start := time.Now()
	run, err := st.ReadRunConsistent(session, budget)
	waited := time.Since(start)
	if err != nil {
		t.Fatalf("ReadRunConsistent with every file held: %v", err)
	}
	if len(run.Declarations) != 1 {
		t.Errorf("got %d declarations, want 1: a read that cannot lock still reads", len(run.Declarations))
	}
	if waited < budget {
		t.Errorf("ReadRunConsistent returned after %v, before its %v budget: it did not wait for the locks", waited, budget)
	}
	// Three per-file budgets would be at least 90 ms; one budget and the
	// scheduler's slack stay under two.
	if waited >= 2*budget {
		t.Errorf("ReadRunConsistent waited %v with three files held, for a %v budget: each file waited a budget of its own", waited, budget)
	}
}
