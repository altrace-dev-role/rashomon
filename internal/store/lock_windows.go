//go:build windows

package store

import (
	"errors"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002

	// syscall defines ERROR_IO_PENDING and not this one.
	errorLockViolation = syscall.Errno(33)

	// A lock covers a byte range, not a file, so a range bounded by the size
	// at acquisition would leave everything appended after it unlocked. The
	// documented way to cover the whole file however large it grows is to lock
	// the maximum range.
	lockBytesLow  = 0xFFFFFFFF
	lockBytesHigh = 0xFFFFFFFF
)

// LockFile keeps lock_unix.go's contract on Windows: an exclusive lock on f,
// polled so the wait can be bounded the way lock_unix.go bounds it. LockFileEx
// without LOCKFILE_FAIL_IMMEDIATELY blocks in the kernel with no deadline, and
// this process must not start a goroutine it would then have to abandon.
//
// These locks are mandatory rather than advisory: while one is held, every
// other process is denied read as well as write access to the range.
//
// It returns how long it waited, as lock_unix.go's does (perf-7).
func lockFileWait(f *os.File, budget time.Duration) (func(), time.Duration, error) {
	h := syscall.Handle(f.Fd())
	start := time.Now()
	deadline := start.Add(budget)
	backoff := 200 * time.Microsecond
	// waited is how long this call has spent on the lock, which it reports
	// (perf-7): nothing when the first try takes it.
	var waited time.Duration

	for {
		var overlapped syscall.Overlapped
		r1, _, err := procLockFileEx.Call(uintptr(h),
			lockfileExclusiveLock|lockfileFailImmediately, 0,
			lockBytesLow, lockBytesHigh, uintptr(unsafe.Pointer(&overlapped)))
		if r1 != 0 {
			return func() { unlockFile(f) }, waited, nil
		}
		if !errors.Is(err, errorLockViolation) && !errors.Is(err, syscall.ERROR_IO_PENDING) {
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
// the file's lock. LockFileEx locks are mandatory: ReadFile on a range
// another handle has locked fails with ERROR_LOCK_VIOLATION.
func readRefused(err error) bool { return errors.Is(err, errorLockViolation) }

// unlockFile releases f's lock through f itself, not through its handle
// value: once f is closed, Control refuses to run, so a late unlock cannot
// release the lock of a file that has been given the value since (CWE-672).
// An unlock that fails leaves nothing to undo: closing the handle, which
// every caller does next, releases the lock.
func unlockFile(f *os.File) {
	rc, err := f.SyscallConn()
	if err != nil {
		return
	}
	_ = rc.Control(func(fd uintptr) {
		var overlapped syscall.Overlapped
		_, _, _ = procUnlockFileEx.Call(fd, 0,
			lockBytesLow, lockBytesHigh, uintptr(unsafe.Pointer(&overlapped)))
	})
}
