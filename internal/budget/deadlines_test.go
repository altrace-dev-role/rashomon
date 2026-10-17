package budget

import (
	"testing"
	"time"
)

// The entry-level soft deadlines of Technical spec 8.2.5 / archive 85.3.10,
// in the table's order. A literal copy, so a changed number shows up here
// as a contract change rather than a silent drift between the two
// documents.
var wantSoftBudgets = map[DeadlineEntry]time.Duration{
	EntryHook:               50 * time.Millisecond,
	EntryPost:               150 * time.Millisecond,
	EntryEventPostToolBatch: 100 * time.Millisecond,
	EntryEventOther:         50 * time.Millisecond,
	EntryRecapPending:       200 * time.Millisecond,
	EntryRecapStop:          2 * time.Second,
	EntryProbeStart:         1200 * time.Millisecond,
	EntryProbeEnd:           1000 * time.Millisecond,
}

func TestRES26_SoftBudgetMatchesTheTable(t *testing.T) {
	seen := map[DeadlineEntry]bool{}
	for e, want := range wantSoftBudgets {
		if got := e.SoftBudget(); got != want {
			t.Errorf("DeadlineEntry(%d).SoftBudget() = %v, want %v", e, got, want)
		}
		seen[e] = true
	}
	for _, e := range Entries() {
		if !seen[e] {
			t.Errorf("DeadlineEntry(%d) has no expectation in this test; the table grew without a test", e)
		}
	}
	if got := DeadlineEntry(len(Entries())).SoftBudget(); got != 0 {
		t.Errorf("an entry past the table has a nonzero soft budget: %v", got)
	}
}

// TestRES26_SoftBudgetNeverExceedsTheHardStopMargin moved to
// internal/supervise.TestRES26_SoftBudgetNeverExceedsTheRealHardStop,
// which checks each entry's soft budget against the real HardStop
// computation for its own event rather than an installedS/shortSession
// pair invented per case (the old cases checked EntryProbeEnd against an
// invented 1.9s hard stop; the real one is 1.4s). budget cannot import
// supervise (supervise already imports budget), so the replacement lives
// where HardStop does.

func TestRES26_ProbeStartOrderIsTheFiveFixedPhasesInOrder(t *testing.T) {
	want := []Phase{Labels, Mining, Evict, Compact, Briefing}
	got := ProbeStartOrder()
	if len(got) != len(want) {
		t.Fatalf("%d phases, want %d", len(got), len(want))
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("ProbeStartOrder()[%d] = %v, want %v", i, got[i], p)
		}
	}
}

func TestRES26_ProbeStartFixedBudgetMatchesTheTable(t *testing.T) {
	want := map[Phase]time.Duration{
		Labels:   50 * time.Millisecond,
		Mining:   300 * time.Millisecond,
		Evict:    500 * time.Millisecond,
		Compact:  200 * time.Millisecond,
		Briefing: 150 * time.Millisecond,
	}
	var sum time.Duration
	for p, d := range want {
		if got := ProbeStartFixedBudget(p); got != d {
			t.Errorf("ProbeStartFixedBudget(%v) = %v, want %v", p, got, d)
		}
		sum += d
	}
	// A phase outside the fixed five has no fixed budget.
	if got := ProbeStartFixedBudget(CostScan); got != 0 {
		t.Errorf("ProbeStartFixedBudget(CostScan) = %v, want 0", got)
	}
	if sum != 1200*time.Millisecond {
		t.Fatalf("the five fixed phases sum to %v, want exactly the entry's 1.2s so the test below's cap is the binding one", sum)
	}
}

// TestRES26_ProbeStartFixedBudgetGuardsAPhasePastTheEnd pins
// ProbeStartFixedBudget's bounds guard: probeStartFixed is now a fixed
// array indexed by Phase (not a map), so a Phase at or past phaseCount
// would index out of range and panic without the guard, in the style of
// DeadlineEntry.SoftBudget's own.
func TestRES26_ProbeStartFixedBudgetGuardsAPhasePastTheEnd(t *testing.T) {
	if got := ProbeStartFixedBudget(Phase(phaseCount)); got != 0 {
		t.Errorf("ProbeStartFixedBudget(phaseCount) = %v, want 0", got)
	}
	if got := ProbeStartFixedBudget(Phase(255)); got != 0 {
		t.Errorf("ProbeStartFixedBudget(255) = %v, want 0", got)
	}
}

func TestRES26_ProbeStartCostScanBudgetIsWhateverIsLeftCappedAt50ms(t *testing.T) {
	cases := []struct {
		name  string
		spent time.Duration
		want  time.Duration
	}{
		{"nothing spent yet: capped at 50ms, not the 1.2s left", 0, 50 * time.Millisecond},
		{"1.19s spent: 10ms left, under the cap", 1190 * time.Millisecond, 10 * time.Millisecond},
		{"exactly at the entry deadline: nothing left", 1200 * time.Millisecond, 0},
		{"over the entry deadline: never negative", 1300 * time.Millisecond, 0},
	}
	for _, c := range cases {
		if got := ProbeStartCostScanBudget(c.spent); got != c.want {
			t.Errorf("%s: ProbeStartCostScanBudget(%v) = %v, want %v", c.name, c.spent, got, c.want)
		}
	}
}

// There is no test here named for RES-26's own acceptance line
// (TestRES26_OptionalPhasesStandDownAtTheDeadline, "the record lands, exit
// 0, wall <= deadline + 50ms") because that needs a real optional phase
// inside a real hook invocation, and none exists in this tree yet (budget
// is contract-only per the package doc; the deadlines are wired by the PRs
// that own each phase). RES-26 is therefore only partially met here: the
// table and its arithmetic are pinned above, and
// internal/supervise.TestChildSoftClockStandsDownAtItsDeadline demonstrates
// the stand-down shape against the real production seam (Child.SoftClock),
// but neither is the named acceptance test, and this file does not claim
// to be.
