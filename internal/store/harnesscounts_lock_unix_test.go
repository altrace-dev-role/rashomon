//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// RecordHarnessMismatch's give-up under a held lock, on the exact clock
// pollNow and pollSleep give a test (lock_unix.go), so it is unix-only; the
// Windows lock contract job's list never named it. On the real clock it
// measured wall time against a hold that ends on a timer, and a test
// goroutine stalled past the hold took the released lock and counted
// (LOCK-TIMEOUT1: "counted = true while the file's lock was held by another
// writer" in 0.34 s, CI run 41472719644 attempt 1).

// TestRecordHarnessMismatch_LockTimeoutSkipsWithoutError: a held lock makes
// the write give up inside its budget, reporting counted=false and err=nil
// (6.2.1: "on a timeout the count is skipped and retried later in the
// session") -- never an error a caller would have to decide whether to
// surface, and never a wait past the budget. The wait runs on an exact clock
// that moves only as far as each sleep asks
// (TestRubricPERF1_AGiveUpLandsAtItsBudgetOnAnExactClock's), so the give-up
// lands at harnessCountBudget exactly, whatever the load. The hold stays on
// the real clock only as a backstop: a wait that blocked instead of polling
// takes the lock when the hold ends and counts, rather than hanging the test.
func TestRecordHarnessMismatch_LockTimeoutSkipsWithoutError(t *testing.T) {
	root := t.TempDir()
	// The sidecar, not FileHarnessCounts itself: RecordHarnessMismatch locks
	// FileHarnessCountsLock (see its own doc), so that is the file a test
	// simulating contention has to hold.
	lockPath := filepath.Join(root, FileHarnessCountsLock)
	if err := os.MkdirAll(root, dirMode); err != nil {
		t.Fatal(err)
	}
	holdFor(t, lockPath, 10*time.Second)

	var elapsed time.Duration
	base := time.Now()
	pollNow = func() time.Time { return base.Add(elapsed) }
	pollSleep = func(d time.Duration) {
		elapsed += d
		if elapsed > 10*harnessCountBudget {
			t.Fatalf("the wait has slept %v on an exact clock without giving up, ten times its %v budget", elapsed, harnessCountBudget)
		}
	}
	t.Cleanup(func() { pollNow, pollSleep = time.Now, time.Sleep })

	counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, time.Now())
	if err != nil {
		t.Fatalf("RecordHarnessMismatch under a held lock returned an error: %v", err)
	}
	if counted {
		t.Fatal("counted = true while the file's lock was held by another writer")
	}
	if elapsed != harnessCountBudget {
		t.Errorf("gave up at %v on an exact clock, want its %v budget", elapsed, harnessCountBudget)
	}
}
