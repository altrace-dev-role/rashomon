//go:build unix || windows

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNoteLockWaitRespectsTheSharedDeadline: noteLockWait's own lock wait
// must come out of the same budget LimitLockWaits gives the rest of an
// invocation's appends, not spillBudget on its own -- otherwise a note
// behind a held lock can run the invocation past its deadline by up to
// spillBudget after everything else has already given up.
func TestNoteLockWaitRespectsTheSharedDeadline(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-x"
	st.lockWaited.Store(int64(time.Millisecond)) // makes noteLockWait try to write

	path := filepath.Join(st.RunDir(session), FileLockWait)
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	holdFor(t, path, 200*time.Millisecond)

	st.LimitLockWaits(5 * time.Millisecond)
	start := time.Now()
	st.noteLockWait(session)
	elapsed := time.Since(start)
	if elapsed >= spillBudget {
		t.Errorf("noteLockWait behind a held lock with a 5ms deadline took %v, want it bounded near the deadline rather than the full %v spillBudget", elapsed, spillBudget)
	}
}
