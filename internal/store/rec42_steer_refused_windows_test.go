package store

import (
	"path/filepath"
	"testing"
)

// TestREC42h_ASteerReadRefusedByAHeldLockCountsAsSkipped: AppendSteer holds
// the steer lock while build runs, and LockFileEx locks are mandatory, so an
// unlocked ReadSteer from another handle is refused. The refusal counts as
// one line that could not be read, not a failed read.
//
// It runs only on Windows, in CI's lock-windows job.
func TestREC42h_ASteerReadRefusedByAHeldLockCountsAsSkipped(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const session = "sess-steer-refused"
	if err := st.AppendSteer(session, fixed(steerLines("a"))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.RunDir(session), FileSteer)
	holder := openForAppend(t, path)
	unlock, _, err := lockFile(holder, lockBudget)
	if err != nil {
		t.Fatalf("taking the steer lock: %v", err)
	}
	defer unlock()

	lines, skipped, err := st.ReadSteer(session)
	if err != nil {
		t.Fatalf("ReadSteer under a held mandatory lock: %v, want the refused read counted as skipped", err)
	}
	if skipped != 1 || len(lines) != 0 {
		t.Errorf("got %d lines and %d skipped, want the refused read counted as one skipped line and nothing read", len(lines), skipped)
	}
}
