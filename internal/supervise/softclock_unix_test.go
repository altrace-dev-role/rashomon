//go:build unix

package supervise

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// TestChildSoftClockSharesTheParentsStart is RES-26's supervised case: a
// phase's soft deadline must count from the parent's process start that the
// Child already carries for the hard stop, not from whenever the phase
// happens to ask for a soft Clock.
func TestChildSoftClockSharesTheParentsStart(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	c := &Child{clock: budget.NewClock(start, 10*time.Second)}

	soft := c.SoftClock(budget.EntryHook)
	want := budget.EntryHook.SoftBudget()

	// Left(start) is exactly the entry's soft budget only if soft's own
	// start is the parent's start; if SoftClock restarted the clock at
	// call time instead, Left(start) would be 0 (start is 2023, long
	// before "now").
	if got := soft.Left(start); got != want {
		t.Errorf("Left(start) = %v, want %v: SoftClock did not share the parent's start", got, want)
	}
}

// TestChildSoftClockStandsDownAtItsDeadline demonstrates RES-26's stand-down
// shape against the real production seam a phase would use: a Clock built
// by Child.SoftClock from a real supervised Child, checked between steps,
// stops at the first check past the deadline and never gates the required
// phase beside it. It is not the row's own named acceptance test
// (TestRES26_OptionalPhasesStandDownAtTheDeadline): that needs a real
// optional phase inside a real hook invocation, which does not exist yet.
// Unlike a version of this test that builds its own Clock from
// EntryHook.SoftBudget() directly, this one goes through SoftClock, so a
// regression that stops SoftClock from sharing the parent's start (the bug
// this file's other test pins) would also show up here as a deadline that
// never arrives, because the parent's start used below is seconds in the
// past.
func TestChildSoftClockStandsDownAtItsDeadline(t *testing.T) {
	start := time.Now().Add(-5 * time.Second) // the parent "started" 5s ago
	c := &Child{clock: budget.NewClock(start, 10*time.Second)}
	clock := c.SoftClock(budget.EntryHook) // EntryHook's soft budget is 50ms

	requiredRan := false
	runRequired := func() { requiredRan = true } // never checks the clock

	var stepsRun int
	var reason budget.Reason
	stoodDown := false
	slowStep := func() { time.Sleep(20 * time.Millisecond) } // far slower than a 50ms budget allows, but the clock's start is already 5s in the past, so the very first check is past the deadline
	for i := 0; i < 10 && !stoodDown; i++ {
		if clock.Expired(time.Now()) {
			reason = budget.Steer.Reason() // stands for whichever phase owns this loop
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
		t.Fatalf("the phase never stood down; the parent's start was already 5s in the past against a 50ms soft budget")
	}
	if stepsRun != 0 {
		t.Errorf("stepsRun = %d, want 0: SoftClock's start was not the parent's, or the deadline was not already past", stepsRun)
	}
	if reason != "deadline" {
		t.Errorf("reason = %q, want the closed word %q", reason, "deadline")
	}
}
