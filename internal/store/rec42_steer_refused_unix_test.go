//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestREC42h_ARefusedSteerReadCountsAsSkipped is the unix half of
// TestREC42h_ASteerReadRefusedByAHeldLockCountsAsSkipped. A held flock never
// refuses a read here, so steer.ndjson is a directory and the store counts
// EISDIR as a refusal (test-11's stand-in): ReadSteer under a held steer
// lock reads no lines, one skipped, and does not fail.
func TestREC42h_ARefusedSteerReadCountsAsSkipped(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.readRefusal = refusesAsEISDIR
	const session = "sess-steer-refused"
	path := filepath.Join(st.RunDir(session), FileSteer)
	if err := os.MkdirAll(path, dirMode); err != nil {
		t.Fatal(err)
	}
	holder, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close() //nolint:errcheck // read-only
	unlock, _, err := lockFileWait(holder, lockBudget)
	if err != nil {
		t.Fatalf("taking the steer lock: %v", err)
	}
	defer unlock()

	lines, skipped, err := st.ReadSteer(session)
	if err != nil {
		t.Fatalf("ReadSteer with its read refused: %v, want the refusal counted", err)
	}
	if skipped != 1 || len(lines) != 0 {
		t.Errorf("got %d lines and %d skipped, want none read and the refused read counted as one", len(lines), skipped)
	}
}
