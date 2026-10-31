//go:build unix

package store

import (
	"path/filepath"
	"testing"
	"time"
)

// TestRubricPERF1_AGiveUpLandsAtItsBudgetOnAnExactClock: the same property as
// TestRubricPERF1_AGiveUpLandsAtItsBudget, with the wait's clock and sleep
// replaced by a clock that moves exactly as far as each sleep asks. The real
// test measures wall time, and under load every sleep overshoots, so the
// deadline can fall inside an earlier sleep and an uncut last one is never
// reached: a break that removes the cut then passed (the sweep's NOT DETECTED
// of run 40960774628). Here the sleeps are 0.2, 0.4, ... 6.4 ms, 12.6 ms in
// all; a 17 ms budget leaves 4.4 ms, and the last sleep must be that, so the
// give-up lands at 17 ms exactly. An uncut sleep is 12.8 ms and lands at 25.4.
func TestRubricPERF1_AGiveUpLandsAtItsBudgetOnAnExactClock(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileRecords)
	unlock, _, err := lockFileWait(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatalf("LockFile on an uncontended file: %v", err)
	}
	defer unlock()

	var elapsed time.Duration
	base := time.Now()
	pollNow = func() time.Time { return base.Add(elapsed) }
	pollSleep = func(d time.Duration) { elapsed += d }
	t.Cleanup(func() { pollNow, pollSleep = time.Now, time.Sleep })

	const budget = 17 * time.Millisecond
	release, waited, err := lockFileWait(openForAppend(t, path), budget)
	if err == nil {
		release()
		t.Fatal("a second descriptor took the lock while the first held it")
	}
	if elapsed != budget {
		t.Errorf("contended LockFile gave up at %v on an exact clock for a %v budget: the last polling sleep was not cut to the time left", elapsed, budget)
	}
	if waited != budget {
		t.Errorf("contended LockFile reports it waited %v, want its %v budget", waited, budget)
	}
}
