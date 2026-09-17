//go:build unix

package store

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// errLockTimeout is returned when the lock could not be taken inside the
// budget. It is a give-up, and the caller records it rather than hiding it.
var errLockTimeout = errors.New("store: timed out waiting for the append lock")

// lockBudget bounds the wait well under the 5-second hook timeout.
//
// There is a real tension here: returning promptly under contention pulls
// against landing every record. This is the choice, stated plainly -- wait up
// to two seconds, because an append is a sub-millisecond operation and sixty-
// four of them serialise in single-digit milliseconds, so reaching this budget
// means something is wrong rather than merely busy. On give-up the caller still
// writes a terminal record, so a dropped declaration is visible as a gap rather
// than as a success.
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
			return nil, errLockTimeout
		}
		time.Sleep(backoff)
		if backoff < 10*time.Millisecond {
			backoff *= 2
		}
	}
}
