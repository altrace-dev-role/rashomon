//go:build unix || windows

package store

import (
	"path/filepath"
	"testing"
	"time"
)

// TestRubricPERF1_AGiveUpLandsAtItsBudget: a contended LockFile gives up when
// its budget runs out, not up to a whole polling sleep later. The polling
// sleep doubles to 12.8 ms, and a wait that checked the deadline only before
// sleeping gave up at the end of whichever sleep crossed it: by the sleeps'
// own lengths a 10 ms budget gave up at 12.6 ms and a 17 ms one at 25.4 ms,
// and on darwin, where each sleep also overshoots by about half a
// millisecond, at 15.8 ms and about 29 ms. digest's read budget is only worth
// what it says when the give-up lands where the budget does, so the last
// sleep is cut to the time left and the lock is tried once more at the
// deadline.
func TestRubricPERF1_AGiveUpLandsAtItsBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileRecords)
	unlock, err := LockFile(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatalf("LockFile on an uncontended file: %v", err)
	}
	defer unlock()

	// 17 ms sits past the first six sleeps on both platforms (12.6 ms, about
	// 15.8 on darwin) and early in the seventh, so an uncut seventh sleep
	// runs at least 8 ms past it. The slack is for the scheduler, and under
	// that overshoot.
	const budget = 17 * time.Millisecond
	const slack = 6 * time.Millisecond
	start := time.Now()
	release, err := LockFile(openForAppend(t, path), budget)
	waited := time.Since(start)
	if err == nil {
		release()
		t.Fatal("a second descriptor took the lock while the first held it")
	}
	if waited < budget {
		t.Errorf("contended LockFile gave up after %v, before its %v budget", waited, budget)
	}
	if waited > budget+slack {
		t.Errorf("contended LockFile gave up after %v for a %v budget: the last polling sleep ran past the deadline", waited, budget)
	}
}
