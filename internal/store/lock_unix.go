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
// to two seconds. An ordered append holds the lock for its seq counter and its
// record, each put behind a barrier rather than synced in full, because the
// full sync is the hook's one Flush, taken after the lock is released
// (REC-34). Measured on an Apple M3 under load, that hold is 0.72 ms at p50,
// 1.3 ms at p90 and 3.6 ms at p99, so the 192 ordered appends of sixty-four
// hook-and-post pairs hold the lock for about 140 ms in all at the median
// and under 0.7 s if every one took the p99. That counts the holds alone:
// the polling backoff below leaves the lock idle between holders, and a
// barrier waits behind another process's full sync, which drains the
// device's queue. The whole burst, measured (RES-30, 2026-10-13), takes 0.49
// to 0.59 s with no lock timeout, so reaching this budget means something is
// wrong rather than merely busy. With the full sync inside the lock the same
// hold was 6.9, 7.9 and 11.6 ms, and the holds alone came to 1.3 s at the
// median and 2.2 s at p99, which is how H-16 failed under load. Linux and Windows have
// no barrier (sync_other.go), so there the hold still includes a full sync.
// On give-up the caller writes a terminal record to the spill file, which
// needs no lock, so the dropped declaration is visible under its own
// tool_use_id rather than as a success.
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
