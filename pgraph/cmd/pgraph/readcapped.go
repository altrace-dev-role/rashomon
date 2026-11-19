package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"syscall"
)

// errOverCap is what readCapped's error matches under errors.Is for a
// regular file over the cap: by its size at the open, or grown past the cap
// while it was read. The error is a *fs.PathError whose Err is an
// *overCapError, which holds the size and the cap, so a caller can say how
// large the file is and never call it corrupt (a read cut at the cap would
// decode as a truncated log).
//
// This reader is the twin of rashomon's regfile.ReadCapped
// (internal/regfile/regfile.go, ErrOverCap and OverCapError): pgraph is its
// own module and imports nothing of rashomon's, so it is spelled again here.
// The two differ in one rule: regfile's follows a link, this one refuses any
// link. A change to one is a change to both.
var errOverCap = errors.New("over the cap")

// errNotRegular is readCapped's refusal of anything but a regular file: a
// link, a directory, a named pipe or a device.
var errNotRegular = errors.New("not a regular file")

// errReplaced is readCappedAt's refusal of a file that is not the one its
// Lstat described: the name was given to another file in between.
var errReplaced = errors.New("replaced between its check and its open")

// overCapError is a capped read refused for its size. Size is the
// descriptor's size at the open, or, for a file that grew past Max while it
// was read, the Max+1 bytes read when the read was refused: at least that.
type overCapError struct {
	Size, Max int64
}

func (e *overCapError) Error() string {
	return fmt.Sprintf("%d bytes, over the %d byte cap", e.Size, e.Max)
}

// Is makes an *overCapError match errOverCap.
func (e *overCapError) Is(target error) bool { return target == errOverCap }

// readCapped is the whole of the regular file at path, refused over max
// bytes; no bytes come with a refusal. A link is refused, never followed:
// the files under .pgraph are pgraph's own and are never links (writeAtomic
// renames a new file over the name), so a link there is someone else's, and
// the vouch read of current must not read through one. The Lstat's own
// error comes back as it is, so a missing file still matches
// fs.ErrNotExist.
//
// Cost: one Lstat, then readCappedAt's open, fstat and read.
func readCapped(path string, max int64) ([]byte, error) {
	lfi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !lfi.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "open", Path: path, Err: errNotRegular}
	}
	return readCappedAt(path, max, lfi)
}

// readCappedAt reads path as readCapped does, given lfi, the Lstat that
// found it a regular file. The Lstat alone does not keep a named pipe out:
// one can take the name before the open. So the open does not wait
// (O_NONBLOCK: a pipe with no writer opens at once, and a regular file reads
// as it would without the flag), and the descriptor's own fstat must show a
// regular file that is the one lfi describes (os.SameFile).
func readCappedAt(path string, max int64, lfi fs.FileInfo) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	fi, err := f.Stat()
	switch {
	case err != nil:
		return nil, err
	case !fi.Mode().IsRegular():
		return nil, &fs.PathError{Op: "open", Path: path, Err: errNotRegular}
	case !os.SameFile(lfi, fi):
		return nil, &fs.PathError{Op: "open", Path: path, Err: errReplaced}
	case fi.Size() > max:
		return nil, &fs.PathError{Op: "open", Path: path, Err: &overCapError{Size: fi.Size(), Max: max}}
	}
	return readAllCapped(f, path, max)
}

// readAllCapped is all of r, refused once it passes max bytes. It reads at
// most max+1 bytes, and that last byte refuses the read with no bytes, so a
// file that grew after its fstat is never handed back cut at the cap and
// read as whole. path names the file in the error.
func readAllCapped(r io.Reader, path string, max int64) ([]byte, error) {
	// One byte past the cap is asked for so that it can be seen; at
	// max = math.MaxInt64 there is no byte past it, and adding one would
	// overflow.
	n := max
	if n < math.MaxInt64 {
		n++
	}
	b, err := io.ReadAll(io.LimitReader(r, n))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, &fs.PathError{Op: "read", Path: path, Err: &overCapError{Size: max + 1, Max: max}}
	}
	return b, nil
}
