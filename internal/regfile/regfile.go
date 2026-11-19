// Package regfile opens a file for reading only when it is a regular one, so
// a named pipe, a socket, a device or a directory at a path rashomon reads
// never holds the read: a hook that waited on one would wait every turn
// (STORE-READ-REGULAR1). It writes nothing; it lives apart from internal/store
// so TRU-23's write-site count stays a count of writes.
package regfile

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
)

// ErrNotRegular is what Open's error matches under errors.Is for a
// path that is there and is not a regular file: a named pipe, a socket, a
// device or a directory.
var ErrNotRegular = errors.New("not a regular file")

// ErrNotDir is what OpenDir's error matches under errors.Is for a path that
// is there and is not a directory.
var ErrNotDir = errors.New("not a directory")

// ErrOverBound is what ReadFile's and ReadBounded's error matches under
// errors.Is for a file holding more bytes than the bound its caller set.
var ErrOverBound = errors.New("over the bound")

// ReadFile is os.ReadFile for a regular file of at most limit bytes: Open's
// refusals, then ReadBounded on the size the open's own fstat reported, so
// it costs what os.ReadFile cost (an open, one fstat, one buffer) and a hook
// that reads through it allocates no more than it did (BenchmarkRunHook). A
// file over limit is a *fs.PathError wrapping ErrOverBound, and no byte of it
// is returned.
func ReadFile(path string, limit int64) ([]byte, error) {
	f, fi, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	b, err := ReadBounded(f, fi.Size(), limit)
	if err != nil {
		return nil, &fs.PathError{Op: "read", Path: path, Err: err}
	}
	return b, nil
}

// ReadBounded reads r to its end when that is at most limit bytes, and
// otherwise returns ErrOverBound and nothing it read. size is what the
// file's own fstat reported: a size over limit is refused before any read,
// and the buffer is sized as os.ReadFile sizes its own (size+1, so a file
// that has not grown ends on one read; 512 at least, since a file under
// /proc reports 0), so it is one allocation. The bound is checked again on
// every read, so a file that grew past limit after its fstat is refused too.
func ReadBounded(r io.Reader, size, limit int64) ([]byte, error) {
	if size > limit {
		return nil, ErrOverBound
	}
	n := int(size) + 1
	if n < 512 {
		n = 512
	}
	data := make([]byte, 0, n)
	for {
		k, err := r.Read(data[len(data):cap(data)])
		data = data[:len(data)+k]
		if int64(len(data)) > limit {
			return nil, ErrOverBound
		}
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return nil, err
		}
		if len(data) == cap(data) {
			data = append(data, 0)[:len(data)]
		}
	}
}

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
	f, _, err := openRegular(path)
	return f, err
}

// openRegular is Open's open and check, returning the mode it read too, so
// ReadFile sizes its buffer from the same fstat; Open and ReadFile share the
// one check.
func openRegular(path string) (*os.File, fs.FileInfo, error) {
	f, fi, err := openNonblocking(path)
	if err == nil && !fi.Mode().IsRegular() {
		err = &fs.PathError{Op: "open", Path: path, Err: ErrNotRegular}
	}
	f, err = unlessRefused(f, err)
	return f, fi, err
}

// OpenDir is Open for a directory a caller lists (card PL-WIRE1: the ledger
// verb looks for a package's test file): the same non-blocking open and the
// same check on the descriptor it opened, so a named pipe swapped in for the
// directory cannot hold it; anything but a directory is closed and refused
// with a *fs.PathError wrapping ErrNotDir.
func OpenDir(path string) (*os.File, error) {
	f, fi, err := openNonblocking(path)
	if err == nil && !fi.IsDir() {
		err = &fs.PathError{Op: "open", Path: path, Err: ErrNotDir}
	}
	return unlessRefused(f, err)
}

// openNonblocking is the one open Open and OpenDir share, O_RDONLY and
// O_NONBLOCK, and the mode of the descriptor it opened; the package's read
// call sites stay one (TRU-23 counts them).
func openNonblocking(path string) (*os.File, fs.FileInfo, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	return f, fi, err
}

// unlessRefused is f, or, where err refused it, nothing, f closed.
func unlessRefused(f *os.File, err error) (*os.File, error) {
	if err != nil {
		if f != nil {
			f.Close() //nolint:errcheck // read-only, and the open is refused
		}
		return nil, err
	}
	return f, nil
}
