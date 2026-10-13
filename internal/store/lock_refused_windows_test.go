package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRubricTEST11_AReadRefusedByAHeldLockCountsAsSkipped is the Windows half
// of TestReadRunConsistent_RespectsAHeldLock. LockFileEx locks are mandatory,
// so the unlocked read a digest falls back to when it cannot get a writer's
// lock is refused there, and ReadRunConsistent returned that refusal as an
// error: digest.Build then failed outright during any concurrent append. A
// refused read now counts as one line that could not be read, the way a
// line caught mid-write counts on unix, so the turn reads unverified rather
// than the digest failing.
//
// It runs only on Windows, in CI's lock-windows job; on unix a held flock
// never refuses a read.
func TestRubricTEST11_AReadRefusedByAHeldLockCountsAsSkipped(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const session = "sess-refused"
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	recordsPath := filepath.Join(dir, FileRecords)
	line := `{"type":"declaration","schema_version":2,"seq":1,"tool_use_id":"toolu_1","session_id":"sess-refused"}` + "\n"
	if err := os.WriteFile(recordsPath, []byte(line), fileMode); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockFile(openForAppend(t, recordsPath), lockBudget)
	if err != nil {
		t.Fatalf("taking the records lock: %v", err)
	}
	defer unlock()

	run, err := st.ReadRunConsistent(session, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("ReadRunConsistent under a held mandatory lock: %v, want the refused file counted as skipped", err)
	}
	if run.Skipped != 1 || len(run.Declarations) != 0 {
		t.Errorf("got %d skipped and %d declarations, want the refused file counted as one skipped line and nothing read",
			run.Skipped, len(run.Declarations))
	}
}
