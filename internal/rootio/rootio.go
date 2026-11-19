// Package rootio writes and reads files through an *os.Root, so a link on a
// path, at its last name or at any directory above it, cannot carry a write
// or a read out of the directory the root names (card WRITE-SAFE1). A link
// that stays inside the root is followed, as an open by path followed it; an
// absolute link is refused even when it points inside (os.Root resolves
// links itself and takes only relative ones). The pgraph module cannot
// import this package, being a module of its own; card READ-SAFE1a gives it
// the same form.
//
// The writer is named as store's is because TRU-23's write-site count
// matches calls by their spelling: each caller's call is counted where it
// is, and the root's own methods, which this package calls, are not a
// spelling that count matches.
package rootio

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// tempTries bounds the search for a free temporary name. A name is 64
// random bits, so a second try is already a thing no run meets; the bound
// is only there so a directory that refuses every create cannot loop.
const tempTries = 100

// ReplaceFile makes name, a path under r, hold exactly what write puts, by
// way of a temporary file beside it renamed over it: a reader sees the old
// file or the new one, never a part of either, and a link at name is
// replaced, not written through. Each missing parent of name is made with
// mode 0755; the file is created with perm under the umask, as
// the create it replaces was, and nothing changes its mode after. A failure
// at any step removes the temporary file and leaves name as it was. The
// error names the target, the root's path joined with name, on one line.
func ReplaceFile(r *os.Root, name string, perm fs.FileMode, write func(io.Writer) error) error {
	if err := replace(r, name, perm, write); err != nil {
		return fmt.Errorf("%s: %w", filepath.Join(r.Name(), name), err)
	}
	return nil
}

func replace(r *os.Root, name string, perm fs.FileMode, write func(io.Writer) error) error {
	dir := filepath.Dir(name)
	if err := r.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, tmp, err := createTemp(r, dir, filepath.Base(name), perm)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()     //nolint:errcheck // the write failed; the temp is removed
		r.Remove(tmp) //nolint:errcheck // best effort: the write's error is the one to report
		return err
	}
	if err := f.Close(); err != nil {
		r.Remove(tmp) //nolint:errcheck // best effort: the close's error is the one to report
		return err
	}
	if err := r.Rename(tmp, name); err != nil {
		r.Remove(tmp) //nolint:errcheck // best effort: the rename's error is the one to report
		return err
	}
	return nil
}

// createTemp creates a new file in dir under r named after base, the form
// store's temporaries take (.<base>.tmp-<random>), with O_EXCL so a name
// already there, a link included, is never opened.
func createTemp(r *os.Root, dir, base string, perm fs.FileMode) (*os.File, string, error) {
	for range tempTries {
		tmp := filepath.Join(dir, "."+base+".tmp-"+strconv.FormatUint(rand.Uint64(), 36))
		f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if !errors.Is(err, fs.ErrExist) {
			return f, tmp, err
		}
	}
	return nil, "", &fs.PathError{Op: "createtemp", Path: filepath.Join(dir, "."+base+".tmp-*"), Err: fs.ErrExist}
}

// ReadRegular reads name, a path under r, when it is a regular file of at
// most limit bytes. It opens with O_NONBLOCK and checks the mode on the
// descriptor it opened, as regfile.Open does, so a named pipe cannot hold
// the read; anything but a regular file is a *fs.PathError wrapping
// regfile.ErrNotRegular. A file over limit is refused, and no byte of it is
// returned, by regfile.ReadCappedFrom's *fs.PathError wrapping a
// *regfile.OverCapError: it matches regfile.ErrOverCap and holds the size
// and the limit, so a caller can record the refusal by its class without
// the path. Every error names the root's path joined with name.
func ReadRegular(r *os.Root, name string, limit int64) ([]byte, error) {
	path := filepath.Join(r.Name(), name)
	f, err := r.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "open", Path: path, Err: regfile.ErrNotRegular}
	}
	b, err := regfile.ReadCappedFrom(f, path, fi.Size(), limit)
	if errors.Is(err, regfile.ErrOverCap) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}
