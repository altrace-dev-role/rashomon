//go:build unix

package store

import (
	"errors"
	"os"
	"syscall"
	"time"
)

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
