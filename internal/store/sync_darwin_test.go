//go:build darwin

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestREC34f_BarrierFallsBackOnlyOnRefusal: the barrier falls back to a full
// sync on the three errors that mean "this filesystem does not do barriers"
// and on nothing else. A real I/O error that fell back would retry the sync
// it had just failed; a refusal that did not fall back would fail every
// append on a volume that is not APFS (REC-34, decided 2026-10-13).
func TestREC34f_BarrierFallsBackOnlyOnRefusal(t *testing.T) {
	for _, c := range []struct {
		errno syscall.Errno
		want  bool
	}{
		{syscall.ENOTSUP, true}, {syscall.EINVAL, true}, {syscall.ENOTTY, true}, {syscall.ENOSYS, true},
		{syscall.EIO, false}, {syscall.EBADF, false}, {syscall.ENODEV, false}, {syscall.ENOSPC, false},
	} {
		if got := barrierRefused(c.errno); got != c.want {
			t.Errorf("barrierRefused(%v) = %v, want %v", c.errno, got, c.want)
		}
	}

	// On the APFS volume the tests run on, the barrier itself succeeds.
	f, err := os.OpenFile(filepath.Join(t.TempDir(), FileRecords), appendFlags, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	durable, err := barrierSync(f)
	if err != nil {
		t.Fatalf("barrierSync on a regular file: %v", err)
	}
	// An accepted barrier orders the write and leaves it in the device's
	// cache: it is the group commit's Flush that makes it durable, so the
	// barrier must not say it already is (P09-F9).
	if durable {
		t.Error("an accepted barrier reported its write durable: Flush would skip the one full sync that makes it so")
	}
}

// TestREC34f_BarrierIsTheKernelsBarrier: the barrier is fcntl's
// F_BARRIERFSYNC, 85 in <sys/fcntl.h>, and not F_FULLFSYNC, which would put
// the device flush back inside every append lock; and it reaches the kernel,
// which refuses a descriptor that is not open, where a barrier that did
// nothing would report success. That it is a barrier and not a full sync on
// the store's own volume is REC-34's acceptance test, through the sync
// recorder, which logs a barrier only once the kernel has accepted one.
func TestREC34f_BarrierIsTheKernelsBarrier(t *testing.T) {
	if fBarrierFsync != 85 || fBarrierFsync == syscall.F_FULLFSYNC {
		t.Errorf("fBarrierFsync = %d; want F_BARRIERFSYNC, 85, never F_FULLFSYNC (%d)", fBarrierFsync, syscall.F_FULLFSYNC)
	}
	// Far above any descriptor limit, so never open.
	notOpen := os.NewFile(1<<20, "not-open")
	_, err := barrierSync(notOpen)
	if !errors.Is(err, syscall.EBADF) {
		t.Errorf("barrierSync on a descriptor that is not open = %v, want EBADF from the kernel", err)
	}
}
