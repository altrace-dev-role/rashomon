package store

import (
	"os"
	"time"
)

// LockFile takes an exclusive lock on f within budget and returns the function
// that releases it. Every file the store locks is locked by the same primitive,
// lockFileWait, and this is exported so that a second package needing a bounded exclusive lock on a file
// of its own takes this one rather than a copy of the three platform files: a
// fix to one copy does not reach the other.
//
// The contract, on every platform that implements it: exclusion is between
// descriptors, so two parts of one process that must exclude each other open
// the file twice (a second call on a descriptor that already holds the lock
// succeeds under flock and fails under LockFileEx, and nothing may rely on
// either); contention that outlasts budget returns ErrLockTimeout, never
// sooner; any other failure returns its own error at once, because the hooks
// record a timeout as lock_timeout and a broken descriptor is not a busy
// lock; f needs read or write access, which LockFileEx demands (appendFlags);
// and the caller releases before it closes f. A budget of 0 is a try-lock:
// one attempt, ErrLockTimeout at once if the lock is held. Where no lock is
// implemented, lock_other.go refuses.
//
// Lock a file kept for the lock alone, not the data it guards. Under
// LockFileEx the lock is mandatory: while it is held, a read or write of the
// locked range through any other descriptor fails, so a reader that never
// takes the lock still cannot read a data file another process has locked on
// Windows, where under flock it reads freely.
//
// It is lockFileWait (lock_unix.go, lock_windows.go, lock_other.go) without
// the wait it reports: the store's own locks go through lockCounted, which
// keeps that figure, and a caller outside the store has no use for it.
func LockFile(f *os.File, budget time.Duration) (func(), error) {
	unlock, _, err := lockFileWait(f, budget)
	return unlock, err
}
