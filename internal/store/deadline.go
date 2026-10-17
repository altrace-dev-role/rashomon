package store

import "time"

// LimitLockWaits gives every lock wait this Store makes from now on one
// deadline, total from now: each wait is its own budget or the time left,
// whichever is shorter, and a wait with no time left tries its lock once.
//
// It is for a hook invocation, which appends up to three times against one
// hook timeout. Each append used to wait out its own two-second lockBudget,
// so a declaration that landed late, its terminal and its coverage record
// could wait six seconds between them against a five-second timeout, and
// the timeout's signal then cut the invocation short at whatever it was
// doing (perf-2). With one deadline the invocation runs out of lock time
// first, and each append's own give-up path -- the spill file for a
// terminal or an execution, an absent coverage record read as unverified --
// still runs inside the timeout.
//
// Call it before the first append. Like DeferDurability, it makes the Store
// one invocation's, not for concurrent use.
func (s *Store) LimitLockWaits(total time.Duration) {
	s.waitUntil = time.Now().Add(total)
}

// lockWait is how long the next lock wait may take: budget, or the time
// left before LimitLockWaits' deadline when that is shorter, and never less
// than zero. A Store without a deadline waits each budget in full.
func (s *Store) lockWait(budget time.Duration) time.Duration {
	if s.waitUntil.IsZero() {
		return budget
	}
	return max(0, min(budget, time.Until(s.waitUntil)))
}
