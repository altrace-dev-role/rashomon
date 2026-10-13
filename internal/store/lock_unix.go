//go:build unix

package store

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// LockFile takes an exclusive lock on f within budget and returns the function
// that releases it. Every file the store locks is locked through it, and it is
// exported so that a second package needing a bounded exclusive lock on a file
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
// Here it is flock, advisory, polled so the wait can be bounded. A blocking
// LOCK_EX cannot be interrupted on a deadline without tearing down the
// goroutine holding it, and this process must not start goroutines it then
// has to abandon.
func LockFile(f *os.File, budget time.Duration) (func(), error) {
	fd := int(f.Fd())
	deadline := time.Now().Add(budget)
	backoff := 200 * time.Microsecond

	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrLockTimeout
		}
		// The last sleep is cut to the time left, so the final try lands at
		// the deadline and a give-up never runs a whole sleep past the
		// budget (perf-1).
		time.Sleep(min(backoff, left))
		if backoff < 10*time.Millisecond {
			backoff *= 2
		}
	}
}
