//go:build darwin

package store

import (
	"os"
	"syscall"

	"github.com/altrace-dev-role/rashomon/internal/fault"
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
// It reports durable false: the write is ordered, not yet on stable storage.
// A filesystem that does not do barriers (the man page names APFS as the only
// one that does) gets the full sync instead, so the fallback is never weaker
// than the call it replaces, and reports durable true, since that sync left
// Flush nothing to do. Any other error is the sync failing, and is returned
// rather than retried as a full sync.
//
// The fault build's sync recorder logs what was issued, not what was meant:
// a barrier once the kernel has accepted one, and the fallback as the full
// sync it is, through syncFullAsBarrier -- logged as SyncBarrierFull, not
// SyncFull, since it runs inside the append's lock a true full sync never
// does. So REC-34's test, which wants cheap barriers inside the lock and one
// full sync outside it, fails if the barrier falls back on the store's own
// volume or never reaches the kernel. A wrong command the kernel accepts,
// F_FULLFSYNC above all, would be logged as a barrier, so TestREC34h_ reads
// the command from the source.
//
// syscall.Syscall on darwin traps straight into the kernel rather than going
// through libc as os.File.Sync does. Apple does not promise that interface;
// fcntl's number in it has not moved, and an error it did return would fail
// the append, never pass it silently.
func barrierSync(f *os.File) (durable bool, err error) {
	return barrierWith(f, fcntlBarrier)
}

// fcntlBarrier issues fcntl(fd, F_BARRIERFSYNC) and returns the kernel's
// answer. It is the only fcntl in the store, which TestREC34h_ checks.
func fcntlBarrier(fd uintptr) syscall.Errno {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, fBarrierFsync, 0)
	return errno
}

// barrierWith is barrierSync with the kernel call passed in, so that a test
// can stand in a filesystem that refuses the barrier: the store's own volume
// is APFS, where the fallback is never reached, and the fallback is the one
// path on which barrierSync says durable and Flush skips its full sync.
func barrierWith(f *os.File, fcntl func(fd uintptr) syscall.Errno) (durable bool, err error) {
	for {
		errno := fcntl(f.Fd())
		switch {
		case errno == 0:
			fault.RecordSync(fault.SyncBarrier, f.Name())
			return false, nil
		case errno == syscall.EINTR:
			continue
		case barrierRefused(errno):
			if err := syncFullAsBarrier(f); err != nil {
				return false, err
			}
			return true, nil
		default:
			return false, errno
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
