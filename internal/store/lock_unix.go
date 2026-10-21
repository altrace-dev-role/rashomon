//go:build unix

package store

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// lockBudget bounds the wait well under the 5-second hook timeout.
//
// There is a real tension here: returning promptly under contention pulls
// against landing every record. This is the choice, stated plainly -- wait up
// to two seconds, because an append is a sub-millisecond operation and sixty-
// four of them serialise in single-digit milliseconds, so reaching this budget
// means something is wrong rather than merely busy. On give-up the caller
// writes a terminal record to the spill file, which needs no lock, so the
// dropped declaration is visible under its own tool_use_id rather than as a
// success.
const lockBudget = 2 * time.Second

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
// and the caller releases before it closes f. Where no lock is implemented,
// lock_other.go refuses.
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
		if time.Now().After(deadline) {
			return nil, ErrLockTimeout
		}
		time.Sleep(backoff)
		if backoff < 10*time.Millisecond {
			backoff *= 2
		}
	}
}
