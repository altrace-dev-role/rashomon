// Package regfile opens a file for reading only when it is a regular one, so
// a named pipe, a socket, a device or a directory at a path rashomon reads
// never holds the read: a hook that waited on one would wait every turn
// (STORE-READ-REGULAR1). It writes nothing; it lives apart from internal/store
// so TRU-23's write-site count stays a count of writes.
package regfile

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// ErrNotRegular is what Open's error matches under errors.Is for a
// path that is there and is not a regular file: a named pipe, a socket, a
// device or a directory.
var ErrNotRegular = errors.New("not a regular file")

// Open opens path for reading when it is a regular file, and otherwise
// closes what it opened and returns a *fs.PathError wrapping ErrNotRegular.
// It opens with O_NONBLOCK, so a named pipe cannot hold the open until a
// writer comes (a hook would wait with it), and it checks the mode on the
// descriptor it opened, not on the name, so nothing swapped in between a
// check and the open is read: the form of internal/brief's module read
// (STORE-READ-REGULAR1). A missing path is the open's own error, matching
// fs.ErrNotExist. A read of a regular file ignores O_NONBLOCK. O_NOFOLLOW is
// left out: the windows build has none, and a link to a regular file reads
// as the file, as os.Open read it.
//
// Cost: the open and one fstat, the syscalls an Lstat then an Open made.
func Open(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err == nil && !fi.Mode().IsRegular() {
		err = &fs.PathError{Op: "open", Path: path, Err: ErrNotRegular}
	}
	if err != nil {
		f.Close() //nolint:errcheck // read-only, and the open is refused
		return nil, err
	}
	return f, nil
}
