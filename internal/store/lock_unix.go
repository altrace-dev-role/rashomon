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
//
// It returns how long it waited, a wait that gave up included, and nothing
// when the first try took the lock, so its caller can make contention
// visible before it costs a record (perf-7, Store.noteLockWait).
func lockFile(f *os.File, budget time.Duration) (func(), time.Duration, error) {
	fd := int(f.Fd())
	start := time.Now()
	deadline := start.Add(budget)
	backoff := 200 * time.Microsecond
	// waited is how long this call has spent on the lock, which it reports
	// (perf-7): nothing when the first try takes it.
	var waited time.Duration

	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }, waited, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, waited, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, waited, ErrLockTimeout
		}
		// The last sleep is cut to the time left, so the final try lands at
		// the deadline and a give-up never runs a whole sleep past the
		// budget (perf-1).
		time.Sleep(min(backoff, left))
		waited = time.Since(start)
		if backoff < 10*time.Millisecond {
			backoff *= 2
		}
	}
}

// readRefused reports whether a read failed because another handle holds
// the file's lock. flock is advisory, so a held lock never refuses a read.
func readRefused(error) bool { return false }
