//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// perf1ThreeHeld is a store whose session has one declaration in its records
// file and every file of its run held by another descriptor until the test
// ends.
func perf1ThreeHeld(t *testing.T) (*Store, string) {
	t.Helper()
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
		unlock, _, err := lockFileWait(openForAppend(t, filepath.Join(dir, name)), lockBudget)
		if err != nil {
			t.Fatalf("taking the %s lock: %v", name, err)
		}
		t.Cleanup(unlock)
	}
	return st, session
}

// TestRubricPERF1_TheBudgetCoversTheWholeRead: ReadRunConsistent's budget is
// for the whole read, not for each of its three files. Per file, three held
// files cost three budgets plus three polling overshoots: digest's 10 ms
// gave up after about 38 ms, three-quarters of the 50 ms the command owes,
// which is why the budget could not grow to wait out a longer hold. With one
// deadline across the read, a single held file may take all of it and three
// held files cost no more.
//
// On the real clock it shows the read waits for the locks and reads anyway.
// That it waits one budget and not three is pinned on the exact clock
// (TestRubricPERF1_TheBudgetCoversTheWholeReadOnAnExactClock): under load
// every polling sleep overshoots, and a whole-module run took this read 71 ms
// for its 30 ms budget, past the two budgets this test once allowed.
//
// Unix-only for the reason TestReadRunConsistent_RespectsAHeldLock is: the
// read past a held lock it falls back to is refused on Windows.
func TestRubricPERF1_TheBudgetCoversTheWholeRead(t *testing.T) {
	st, session := perf1ThreeHeld(t)
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
}

// TestRubricPERF1_TheBudgetCoversTheWholeReadOnAnExactClock: the same read
// on the exact clock pollNow and pollSleep give a test, which moves only as
// far as each sleep asks. The first held file waits the whole budget and the
// other two try their locks once, so the read ends at its budget exactly; a
// budget per file ends it at three. The clock runs an hour ahead of the wall
// clock, so a deadline read from the wall clock has passed on it and the read
// waits nothing, and a wait read from the wall clock would wait an hour,
// which the sleep's guard stops at ten budgets.
func TestRubricPERF1_TheBudgetCoversTheWholeReadOnAnExactClock(t *testing.T) {
	st, session := perf1ThreeHeld(t)
	const budget = 30 * time.Millisecond
	var elapsed time.Duration
	base := time.Now().Add(time.Hour)
	pollNow = func() time.Time { return base.Add(elapsed) }
	pollSleep = func(d time.Duration) {
		elapsed += d
		if elapsed > 10*budget {
			t.Fatalf("the read has slept %v on an exact clock without giving up, ten times its %v budget", elapsed, budget)
		}
	}
	t.Cleanup(func() { pollNow, pollSleep = time.Now, time.Sleep })

	run, err := st.ReadRunConsistent(session, budget)
	if err != nil {
		t.Fatalf("ReadRunConsistent with every file held: %v", err)
	}
	if len(run.Declarations) != 1 {
		t.Errorf("got %d declarations, want 1: a read that cannot lock still reads", len(run.Declarations))
	}
	if elapsed != budget {
		t.Errorf("ReadRunConsistent ended at %v on an exact clock with three files held, want its %v budget: one deadline for the whole read", elapsed, budget)
	}
}
