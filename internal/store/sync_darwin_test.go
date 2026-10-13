//go:build darwin

package store

import (
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
	if err := barrierSync(f); err != nil {
		t.Fatalf("barrierSync on a regular file: %v", err)
	}
}
