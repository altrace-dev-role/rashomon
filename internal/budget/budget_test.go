package budget

import (
	"testing"
	"time"
)

// The thirteen stand-down words of 8.2.2, in its order. A literal copy, so
// that a phase added or a word changed shows up here as a contract change.
var frozenReasons = []string{
	"verify_budget", "deadline", "learn_budget", "label_budget", "mining_budget",
	"evict_budget", "compact_budget", "cost_scan_budget", "digest_budget",
	"output_scan_budget", "spec_budget", "briefing_budget", "join_budget",
}

func TestPhaseReasonsAreTheFrozenList(t *testing.T) {
	phases := Phases()
	if len(phases) != len(frozenReasons) {
		t.Fatalf("%d phases, want %d", len(phases), len(frozenReasons))
	}
	seen := map[Reason]bool{}
	for i, p := range phases {
		if got := p.Reason(); string(got) != frozenReasons[i] {
			t.Errorf("phase %d: reason %q, want %q", i, got, frozenReasons[i])
		}
		if seen[p.Reason()] {
			t.Errorf("reason %q is used by two phases", p.Reason())
		}
		seen[p.Reason()] = true
	}
	if Phase(phaseCount).Reason() != "" {
		t.Error("a phase past the end has a reason; it must have none")
	}
}

func TestClockSoftKeepsStartChangesBudget(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	hardStop := NewClock(start, 10*time.Second)
	soft := hardStop.Soft(100 * time.Millisecond)
	if soft.Expired(start.Add(99 * time.Millisecond)) {
		t.Error("the derived Clock expired before its own 100ms budget")
	}
	if !soft.Expired(start.Add(100 * time.Millisecond)) {
		t.Error("the derived Clock did not expire at its own 100ms budget")
	}
	// The hard-stop Clock it was derived from is unaffected.
	if hardStop.Expired(start.Add(100 * time.Millisecond)) {
		t.Error("Soft mutated the Clock it was called on")
	}
	// Soft shares start with the Clock it was derived from, not time.Now:
	// start is 2023, so if Soft restarted the clock at call time, Left at
	// start would be clamped to 0 instead of the full 100ms budget.
	if got := soft.Left(start); got != 100*time.Millisecond {
		t.Errorf("Left(start) = %v, want 100ms; Soft did not keep the original start", got)
	}
}

func TestClockExpiresAtTheSoftDeadline(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	c := NewClock(start, 100*time.Millisecond)
	if c.Expired(start.Add(99 * time.Millisecond)) {
		t.Error("expired before the deadline")
	}
	if !c.Expired(start.Add(100 * time.Millisecond)) {
		t.Error("not expired at the deadline")
	}
	if got := c.Left(start.Add(40 * time.Millisecond)); got != 60*time.Millisecond {
		t.Errorf("Left = %v, want 60ms", got)
	}
	if got := c.Left(start.Add(time.Second)); got != 0 {
		t.Errorf("Left past the deadline = %v, want 0", got)
	}
}
