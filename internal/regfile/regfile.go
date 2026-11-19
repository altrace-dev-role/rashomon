// Package regfile opens a file for reading only when it is a regular one, so
// a named pipe, a socket, a device or a directory at a path rashomon reads
// never holds the read: a hook that waited on one would wait every turn
// (STORE-READ-REGULAR1). It writes nothing; it lives apart from internal/store
// so TRU-23's write-site count stays a count of writes.
package regfile

import (
	"errors"
	"fmt"
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
// ReadCapped sizes its buffer from the same fstat; Open and ReadCapped share
// the one check.
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

// ErrOverCap is what the error of OpenCapped, ReadCapped and ReadCappedFrom
// matches under errors.Is for a regular file over the cap: by its size at
// the open, or grown past the cap while it was read. The error is a
// *fs.PathError whose Err is an *OverCapError, which holds the size and the
// cap, so a caller can say how large the file is and never call it corrupt
// (a read cut at the cap would decode as a truncated file), and can record
// the refusal by its class without the path. It is a constant of its own
// type; that is its form, not a saving at init: ErrNotRegular and ErrNotDir
// are package-level errors.New values, and the init trace TestRES2_InitBudget
// logs under go1.27.4 shows no line for this package, since the compiler
// lays such a value out statically.
const ErrOverCap capError = "over the cap"

// capError is ErrOverCap's type, a constant error.
type capError string

func (e capError) Error() string { return string(e) }

// OverCapError is a capped open, read or write refused for its size. Size
// is the descriptor's size at the open, or, for a file that grew past Max
// while it was read, the Max+1 bytes read when the read was refused, or, for
// a write, the bytes asked of the writer when it refused: at least that.
type OverCapError struct {
	Size, Max int64
}

func (e *OverCapError) Error() string {
	return fmt.Sprintf("%d bytes, over the %d byte cap", e.Size, e.Max)
}

// Is makes an *OverCapError match ErrOverCap.
func (e *OverCapError) Is(target error) bool { return target == ErrOverCap }

// OpenCapped is Open with a cap of max bytes. A regular file whose size on
// the descriptor is over max is closed and refused, and the reader it
// returns refuses byte max+1 if the file grows while it is read: it hands
// the caller max bytes at most, then an error matching ErrOverCap, never a
// short file ending in io.EOF. Like Open it follows a link, so a device or
// named pipe at the far end is refused as not regular. pgraph is its own
// module and cannot import this one; its form of the same reader
// (pgraph/cmd/pgraph/readcapped.go, readCapped) refuses any link, for its
// vouch read. A change to one is a change to both.
//
// Cost: Open's open and fstat; each Read is one read of the file.
func OpenCapped(path string, max int64) (io.ReadCloser, error) {
	f, fi, err := openNonblocking(path)
	switch {
	case err != nil:
	case !fi.Mode().IsRegular():
		err = &fs.PathError{Op: "open", Path: path, Err: ErrNotRegular}
	case fi.Size() > max:
		err = &fs.PathError{Op: "open", Path: path, Err: &OverCapError{Size: fi.Size(), Max: max}}
	}
	if f, err = unlessRefused(f, err); err != nil {
		return nil, err
	}
	return &capped{f: f, path: path, max: max}, nil
}

// ReadCapped is the whole of the regular file at path, refused over max
// bytes as OpenCapped refuses it; no bytes come with a refusal. It is Open's
// open and check, then ReadCappedFrom on what that open's own fstat
// reported, so it costs what os.ReadFile cost (an open, one fstat, one
// buffer) and a hook that reads through it allocates no more than it did
// (BenchmarkRunHook).
func ReadCapped(path string, max int64) ([]byte, error) {
	f, fi, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	return ReadCappedFrom(f, path, fi, max)
}

// ReadCappedFrom is ReadCapped over f, a file its caller opened, for a
// caller that opens by another route than a path (an *os.Root). fi is what
// f's own fstat reported, so the mode and the size it checks are of the one
// descriptor, and path is the name every refusal gives, so the caller's
// errors and these name the file alike. Anything but a regular file is
// refused before any read, as a *fs.PathError (Op "open") wrapping
// ErrNotRegular, so a named pipe is never read here, whatever flags it was
// opened with: the check is the reader's own, not left to each caller. It
// guards the read only: an open without O_NONBLOCK waits for a writer
// before this is reached. A size over max is refused before any read, as a
// *fs.PathError (Op "open") wrapping an *OverCapError. The rest is read
// through OpenCapped's reader into one buffer sized as os.ReadFile sizes its
// own (size+1, so a file that has not grown ends on one read; 512 at least,
// since a file under /proc reports 0), so a file that grew past max after
// its fstat is refused at byte max+1 (Op "read"), and no byte past max+1 is
// taken from f. No byte comes with a refusal. It does not close f.
func ReadCappedFrom(f *os.File, path string, fi fs.FileInfo, max int64) ([]byte, error) {
	if !fi.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "open", Path: path, Err: ErrNotRegular}
	}
	size := fi.Size()
	if size > max {
		return nil, &fs.PathError{Op: "open", Path: path, Err: &OverCapError{Size: size, Max: max}}
	}
	n := int(size) + 1
	if n < 512 {
		n = 512
	}
	data := make([]byte, 0, n)
	c := capped{f: f, path: path, max: max}
	for {
		k, err := c.Read(data[len(data):cap(data)])
		data = data[:len(data)+k]
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

// capped is the reader of OpenCapped and ReadCappedFrom: n bytes handed out
// so far, never more than max; the read that would pass max is refused and
// stays refused.
type capped struct {
	f    *os.File
	path string
	max  int64
	n    int64
}

func (c *capped) Read(p []byte) (int, error) {
	if c.n > c.max {
		return 0, c.over()
	}
	// Ask for no more than one byte past the cap, so that byte is seen and
	// never handed out. c.max-c.n is 0 or more here, and adding one is safe
	// once it is under len(p), so a cap of math.MaxInt64 does not overflow.
	if room := c.max - c.n; room < int64(len(p)) {
		p = p[:room+1]
	}
	k, err := c.f.Read(p)
	c.n += int64(k)
	if c.n > c.max {
		return k - int(c.n-c.max), c.over()
	}
	return k, err
}

func (c *capped) over() error {
	return &fs.PathError{Op: "read", Path: c.path, Err: &OverCapError{Size: c.max + 1, Max: c.max}}
}

func (c *capped) Close() error { return c.f.Close() }

// openNonblocking is the one open Open, OpenDir and OpenCapped share,
// O_RDONLY and O_NONBLOCK, and the mode of the descriptor it opened; the
// package's read call sites stay one (TRU-23 counts them).
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
