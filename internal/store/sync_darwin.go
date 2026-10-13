//go:build darwin

package store

import (
	"os"
	"syscall"
)

// fBarrierFsync is fcntl's F_BARRIERFSYNC on darwin (<sys/fcntl.h>: 85). The
// syscall package does not name it, and one constant is not worth a module:
// this one has none, and TRU-40 and RES-3 check that it stays that way.
const fBarrierFsync = 85

// barrierSync pushes f's written data to the device and issues a barrier
// behind it: nothing written after this call reaches stable storage before
// what was written before it. It does not wait for the device to empty its
// cache, which is what makes it cheap: that wait is the whole cost of
// f.Sync() on darwin, where Go implements it with F_FULLFSYNC. A later
// F_FULLFSYNC on any file of the same device persists everything a barrier
// ordered before it (fcntl(2)), which is what Store.Flush relies on.
//
// A filesystem that does not do barriers (the man page names APFS as the only
// one that does) gets the full sync instead, so the fallback is never weaker
// than the call it replaces. Any other error is the sync failing, and is
// returned rather than retried as a full sync.
//
// syscall.Syscall on darwin traps straight into the kernel rather than going
// through libc as os.File.Sync does. Apple does not promise that interface;
// fcntl's number in it has not moved, and an error it did return would fail
// the append, never pass it silently.
func barrierSync(f *os.File) error {
	for {
		_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), fBarrierFsync, 0)
		switch {
		case errno == 0:
			return nil
		case errno == syscall.EINTR:
			continue
		case barrierRefused(errno):
			return f.Sync()
		default:
			return errno
		}
	}
}

// barrierRefused reports the errors that mean the filesystem, the device or
// the kernel does not implement the barrier, as opposed to the sync failing.
// ENOSYS is the raw trap itself going away in a later macOS; the full sync
// is then the one call that still exists.
func barrierRefused(errno syscall.Errno) bool {
	return errno == syscall.ENOTSUP || errno == syscall.EINVAL || errno == syscall.ENOTTY || errno == syscall.ENOSYS
}
