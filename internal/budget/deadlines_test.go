package budget

import (
	"testing"
	"time"
)

// The entry-level soft deadlines of CLAUDE.md 8.2.5 / archive 85.3.10, in
// the table's order. A literal copy, so a changed number shows up here as a
// contract change rather than a silent drift between the two documents.
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

func TestRES26_SoftBudgetNeverExceedsTheHardStopMargin(t *testing.T) {
	// The soft deadline is for optional phases inside the entry; it must
	// leave the required phase room to finish before the hard stop, so it
	// is always comfortably under it. The margin used here mirrors the
	// supervisor's own (85.3.10): entries of 5s+ lose 1s, shorter ones
	// lose 100ms at most.
	cases := []struct {
		e            DeadlineEntry
		installedS   int
		shortSession bool
	}{
		{EntryHook, 5, false},
		{EntryPost, 5, false},
		{EntryEventPostToolBatch, 5, false},
		{EntryEventOther, 5, false},
		{EntryRecapPending, 10, false},
		{EntryRecapStop, 10, false},
		{EntryProbeStart, 5, false},
		{EntryProbeEnd, 2, true}, // SessionEnd-style short budget (1.5s default)
	}
	for _, c := range cases {
		hard := time.Duration(c.installedS) * time.Second
		margin := time.Second
		if hard < 5*time.Second {
			margin = 100 * time.Millisecond
		}
		if soft := c.e.SoftBudget(); soft >= hard-margin {
			t.Errorf("%v: soft %v leaves no room before hard stop %v (margin %v)", c.e, soft, hard, margin)
		}
	}
}

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

// TestRES26_OptionalPhaseStandsDownAtItsDeadline is the mechanism RES-26
// requires: given a Clock built from an DeadlineEntry's soft budget, a phase that
// checks it between steps stops at the first check past the deadline,
// records the phase's own closed reason, and the whole run stays bounded
// by the deadline plus a small, fixed margin -- never by how slow the
// fault-injected step is. The required phase (passed separately, standing
// for "declaration", "execution", ...) always completes, because nothing
// here ever gates it on the clock.
func TestRES26_OptionalPhaseStandsDownAtItsDeadline(t *testing.T) {
	start := time.Now()
	clock := NewClock(start, EntryHook.SoftBudget())

	requiredRan := false
	runRequired := func() { requiredRan = true } // never checks the clock

	var stepsRun int
	var reason Reason
	stoodDown := false
	slowStep := func() { time.Sleep(20 * time.Millisecond) } // the fault: far slower than the 50ms budget allows in one shot, every step after the first blows it
	for i := 0; i < 10 && !stoodDown; i++ {
		if clock.Expired(time.Now()) {
			reason = Steer.Reason() // stands for whichever phase owns this loop
			stoodDown = true
			break
		}
		slowStep()
		stepsRun++
	}
	runRequired()

	if !requiredRan {
		t.Error("the required phase never ran: it must never wait on the optional one")
	}
	if !stoodDown {
		t.Fatalf("the phase never stood down; it ran all %d steps inside a 50ms budget with 20ms steps", stepsRun)
	}
	if reason != "deadline" {
		t.Errorf("reason = %q, want the closed word %q", reason, "deadline")
	}
	if elapsed := time.Since(start); elapsed > clock.soft+50*time.Millisecond {
		t.Errorf("wall clock %v exceeded the soft deadline (%v) by more than the 50ms margin", elapsed, clock.soft)
	}
}
