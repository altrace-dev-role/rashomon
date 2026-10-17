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
	unlock, _, err := lockFile(openForAppend(t, recordsPath), lockBudget)
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

// TestRubricTEST11_AGapsReadRefusedByAHeldLockCountsAsSkipped is the Windows
// half of TestRubricTEST11_ARefusedGapsReadCountsAsSkipped. A forget or an
// eviction holds the gaps file's lock while it writes, and ReadGaps reads
// that file unlocked, so on Windows the read is refused and digest.Build
// failed with it. The refusal now counts as one gap line that could not be
// read, which reads every session unverified (REC-33). ForgottenHost still
// fails, after waiting lockBudget for a hold that here never ends: a gaps
// file it cannot read must not read as "nothing forgotten".
//
// It runs only on Windows, in CI's lock-windows job.
func TestRubricTEST11_AGapsReadRefusedByAHeldLockCountsAsSkipped(t *testing.T) {
	root := t.TempDir()
	st, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	gapsPath := filepath.Join(root, FileGaps)
	if err := os.WriteFile(gapsPath, []byte(`{"type":"gap","schema_version":2,"session_id":"sess-gap"}`+"\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	unlock, _, err := lockFile(openForAppend(t, gapsPath), lockBudget)
	if err != nil {
		t.Fatalf("taking the gaps lock: %v", err)
	}
	defer unlock()

	gaps, skipped, err := st.ReadGaps()
	if err != nil {
		t.Fatalf("ReadGaps under a held mandatory lock: %v, want the refused read counted as skipped", err)
	}
	if skipped != 1 || len(gaps) != 0 {
		t.Errorf("got %d gaps and %d skipped, want none read and one skipped", len(gaps), skipped)
	}
	if _, err := st.ForgottenHost(); err == nil {
		t.Error("ForgottenHost under a mandatory lock held past its wait returned no error, want the refused read to fail it")
	}
}

// refuseRecordsRead makes path's read refused the way Windows refuses it: a
// handle of its own holds the records lock until the returned release.
func refuseRecordsRead(t *testing.T, _ *Store, path string) func() {
	t.Helper()
	if err := os.WriteFile(path, []byte(`{"type":"declaration","schema_version":2,"seq":1}`+"\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	unlock, _, err := lockFile(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatalf("taking the records lock: %v", err)
	}
	return unlock
}
