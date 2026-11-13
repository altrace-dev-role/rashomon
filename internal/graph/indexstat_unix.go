//go:build unix

package graph

import "syscall"

// indexAt reports whether p, an index's path, can be stat'd: the call
// os.Stat makes, following links and retrying EINTR as it does, without
// the FileInfo and *PathError os.Stat builds and FindIndex never reads. The
// Stat_t stays on the stack and a missing file's errno is preallocated, so
// a level costs no allocation beyond the path it is given and the
// NUL-terminated copy the call needs (TestFindIndex_AWalkWithNoIndexAllocatesTwoPerDirectory).
func indexAt(p string) bool {
	var st syscall.Stat_t
	for {
		err := syscall.Stat(p, &st)
		if err != syscall.EINTR {
			return err == nil
		}
	}
}
