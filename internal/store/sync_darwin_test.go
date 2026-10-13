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
// nothing would report success. That barrierSync passes this constant, and
// not F_FULLFSYNC, which the kernel would accept and the recorder would log
// as a barrier all the same, is TestREC34h_'s, from the source; that the
// barrier does not fall back on the store's own volume is REC-34's
// acceptance test, through the recorder.
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

// TestREC34f_ARefusedBarrierSyncsInFull: a filesystem that refuses the
// barrier (APFS is the only one that does barriers, so HFS+ or an exFAT
// external disk) gets the full sync in its place, and the fallback says the
// write is durable, since that sync left Flush nothing to do. If it said so
// without syncing, Store.syncAppend would clear the dirty file and Flush
// would issue no F_FULLFSYNC at all: a deferred append on such a volume would
// never be synced and would be reported durable (P09-F9). A sync on a closed
// file fails, which is how this test sees the fallback sync issued: the
// refusal returns that failure, durable false. An I/O error is the sync
// failing and is returned as it is, with no fallback sync behind it; an
// interrupted call is issued again.
func TestREC34f_ARefusedBarrierSyncsInFull(t *testing.T) {
	refuse := func(errno syscall.Errno) func(uintptr) syscall.Errno {
		return func(uintptr) syscall.Errno { return errno }
	}
	open := func(t *testing.T) *os.File {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(t.TempDir(), FileRecords), appendFlags, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		if _, err := f.Write([]byte("{}\n")); err != nil {
			t.Fatal(err)
		}
		return f
	}
	closed := func(t *testing.T) *os.File {
		t.Helper()
		f := open(t)
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		return f
	}
	for _, errno := range []syscall.Errno{syscall.ENOTSUP, syscall.EINVAL, syscall.ENOTTY, syscall.ENOSYS} {
		durable, err := barrierWith(open(t), refuse(errno))
		if err != nil || !durable {
			t.Errorf("a barrier refused with %v = (durable %v, %v); want (true, nil): the full sync in its place is durable", errno, durable, err)
		}
		durable, err = barrierWith(closed(t), refuse(errno))
		if !errors.Is(err, os.ErrClosed) || durable {
			t.Errorf("a barrier refused with %v on a closed file = (durable %v, %v); want (false, %v): the fallback must issue the full sync and return its failure", errno, durable, err, os.ErrClosed)
		}
	}

	durable, err := barrierWith(closed(t), refuse(syscall.EIO))
	if !errors.Is(err, syscall.EIO) || durable {
		t.Errorf("a barrier failing with EIO = (durable %v, %v); want (false, EIO), with no full sync retried behind it", durable, err)
	}

	calls := 0
	interrupted := func(uintptr) syscall.Errno {
		if calls++; calls == 1 {
			return syscall.EINTR
		}
		return 0
	}
	durable, err = barrierWith(open(t), interrupted)
	if err != nil || durable || calls != 2 {
		t.Errorf("a barrier interrupted once = (durable %v, %v) after %d calls; want (false, nil) after 2", durable, err, calls)
	}
}
