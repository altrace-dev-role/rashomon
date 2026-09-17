//go:build windows

package store

import (
	"errors"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const lockBudget = 2 * time.Second

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

// lockFile takes an exclusive lock on f, polling so the wait can be bounded the
// way lock_unix.go bounds it. LockFileEx without LOCKFILE_FAIL_IMMEDIATELY
// blocks in the kernel with no deadline, and this process must not start a
// goroutine it would then have to abandon.
//
// These locks are mandatory rather than advisory: while one is held, every
// other process is denied read as well as write access to the range.
func lockFile(f *os.File, budget time.Duration) (func(), error) {
	h := syscall.Handle(f.Fd())
	deadline := time.Now().Add(budget)
	backoff := 200 * time.Microsecond

	for {
		var overlapped syscall.Overlapped
		r1, _, err := procLockFileEx.Call(uintptr(h),
			lockfileExclusiveLock|lockfileFailImmediately, 0,
			lockBytesLow, lockBytesHigh, uintptr(unsafe.Pointer(&overlapped)))
		if r1 != 0 {
			return func() {
				var overlapped syscall.Overlapped
				_, _, _ = procUnlockFileEx.Call(uintptr(h), 0,
					lockBytesLow, lockBytesHigh, uintptr(unsafe.Pointer(&overlapped)))
			}, nil
		}
		if !errors.Is(err, errorLockViolation) && !errors.Is(err, syscall.ERROR_IO_PENDING) {
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
