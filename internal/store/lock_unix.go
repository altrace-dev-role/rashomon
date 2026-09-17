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

// lockFile takes an exclusive advisory lock on f, polling so the wait can be
// bounded. A blocking LOCK_EX cannot be interrupted on a deadline without
// tearing down the goroutine holding it, and this process must not start
// goroutines it then has to abandon.
func lockFile(f *os.File, budget time.Duration) (func(), error) {
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
