package store

import "time"

// The lock budgets, declared once for every platform: LockFile differs per
// platform (lock_unix.go, lock_windows.go, lock_other.go), and the waits it
// is given do not (docs-8).

// lockBudget bounds the wait well under the 5-second hook timeout.
//
// There is a real tension here: returning promptly under contention pulls
// against landing every record. This is the choice, stated plainly -- wait up
// to two seconds. An ordered append holds the lock for its seq counter and its
// record (writeOrderedLocked), each put behind a barrier rather than synced
// in full on darwin, because the full sync is the hook's one Flush, taken
// after the lock is released (REC-34).
//
// The holds are measured by BenchmarkOrderedAppendHold on the reference
// machine (Apple M3, go1.27.2, three runs at -benchtime 20x, 2026-10-13,
// with other builds sharing the machine). A hook's append on darwin holds
// the lock 1.4 to 1.7 ms at p50 alone, and in a 64-way burst 1.6 to 2.0 ms at
// p50, 2.5 to 2.8 ms at p90 and 5.8 to 9.4 ms at p99. The 192 ordered appends
// of sixty-four hook-and-post pairs then serialise in about 0.4 s at the
// median, and in 1.8 s if every one took the worst p99: inside this budget,
// with little room, which is why one invocation's waits share a deadline
// (LimitLockWaits) and CI's macOS job fails on any lock_timeout at 64-way.
// RES-30 timed the whole burst end to end (2026-10-13): 0.49 to 0.59 s,
// with no lock timeout.
// With the full sync inside the lock -- every append on Linux and Windows,
// which have no barrier (sync_other.go), and an undeferred Store's on
// darwin -- the 64-way hold was 6.7 to 6.8 ms at p50, 7.9 to 8.3 ms at p90
// and 11.1 to 14.1 ms at p99: 1.3 s for that burst at the median, and past
// this budget at p99. Three more runs while other processes loaded the disk
// moved the medians little and the p99s to 7.7 to 41 ms for a hook's append
// and up to 51 ms alone, and 5 of 1,280 appends in 64-way bursts lost to
// this budget: the tail is set by whatever else is using the disk. Linux's
// own holds are reported by the same benchmark in CI's perf-linux job, and
// H-16's 64 hooks land there. The 0.72, 1.3 and 3.6 ms this comment used to
// quote are not what the benchmark measures.
// On give-up the caller writes a terminal record to the spill file, whose
// writers wait only briefly for its lock and then append regardless
// (spillBudget), so the dropped declaration is visible under its own
// tool_use_id rather than as a success.
const lockBudget = 2 * time.Second

// spillBudget is how long a spill write waits for the spill file's lock. It is
// short because the spill path exists for a handler that has already waited
// once; the lock is taken so that forget, which rewrites the spill file under
// it, is not raced, and given up so that no handler ever blocks on it.
const spillBudget = 50 * time.Millisecond

// steerBudget is how long a steer append waits for the steer log's lock
// (Technical spec 4.2.4 step 10). It is short because what waits is a note,
// not a record: on a give-up nothing is said and the decision is recorded as
// lock_timeout (4.2.5), and RES-30b gates the p99 wait below it.
const steerBudget = 150 * time.Millisecond
