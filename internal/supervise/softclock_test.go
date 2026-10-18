package supervise

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// TestNilChildSoftClockFallsBackToNow is RES-26's unsupervised case: with no
// parent to share a start with, the soft Clock starts at the moment it is
// asked for, not an undefined zero value.
func TestNilChildSoftClockFallsBackToNow(t *testing.T) {
	var c *Child
	before := time.Now()
	clock := c.SoftClock(budget.EntryHook)
	after := time.Now()

	d := budget.EntryHook.SoftBudget()
	if clock.Expired(before.Add(d - time.Millisecond)) {
		t.Error("the fallback Clock expired before its own soft budget had elapsed from start")
	}
	if !clock.Expired(after.Add(d)) {
		t.Error("the fallback Clock never expires")
	}
}

// TestNilChildSoftClockPastTheEndHasAlreadyExpired: an entry past the end of
// the table has no budget, which NewClock refuses; the unsupervised Clock for
// it has already expired, as SoftBudget documents, rather than running on.
func TestNilChildSoftClockPastTheEndHasAlreadyExpired(t *testing.T) {
	var c *Child
	e := budget.DeadlineEntry(len(budget.Entries()))
	if !c.SoftClock(e).Expired(time.Now()) {
		t.Error("an entry with no budget got a Clock that has not expired")
	}
}
