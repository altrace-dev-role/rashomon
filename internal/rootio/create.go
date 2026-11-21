package rootio

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CreateFile makes name, a path under r, hold exactly what write puts, only
// where nothing is at name: a file, a directory or a link there, a dangling
// link included, is left as it is, and the error matches fs.ErrExist. The
// bytes go to a temporary file beside name (createTemp's form) under the cap
// ReplaceFile holds them to, and the temporary is published by a hard link
// on r, which refuses a name already there and never follows a link at it,
// so a reader sees no file at name or the whole file, never a part of it.
// The temporary is removed on every return. A crash between its create and
// its removal leaves it beside name as .<base>.tmp-<random>; nothing in
// rashomon removes it, and a person who sees it removes it by hand; a
// removal that fails after the link is a *LeftError naming it, name holding
// the whole file. A link the filesystem refuses fails the create with that
// error and nothing at name: there is no second way of writing. Each missing
// parent of name is made with mode 0755, and a parent that is a link, even
// one that stays inside the root, refuses the create; a refusal removes the
// parents it made, so the root lists what it listed before. The file is
// created with perm under the umask, and its bytes are synced before the
// link, so a crash after the link cannot leave name holding a part. The
// error names the target, the root's path joined with name, on one line.
func CreateFile(r *os.Root, name string, perm fs.FileMode, max int64, write func(io.Writer) error) error {
	if err := create(r, name, perm, max, write); err != nil {
		return fmt.Errorf("%s: %w", filepath.Join(r.Name(), name), err)
	}
	return nil
}

func create(r *os.Root, name string, perm fs.FileMode, max int64, write func(io.Writer) error) error {
	dir := filepath.Dir(name)
	made, err := parents(r, dir)
	if err != nil {
		unmake(r, made)
		return err
	}
	f, tmp, err := createTemp(r, dir, filepath.Base(name), perm)
	if err != nil {
		unmake(r, made)
		return err
	}
	cw := &capWriter{f: f, max: max}
	err = cw.end(write(cw))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = publish(r, tmp, name)
	}
	if err != nil {
		r.Remove(tmp) //nolint:errcheck // best effort: the step's error is the one to report
		unmake(r, made)
		return err
	}
	if err := discard(r, tmp); err != nil {
		return &LeftError{Temp: tmp, Err: err}
	}
	return nil
}

// errLinkParent is the refusal of a parent of name that is a link.
var errLinkParent = errors.New("is a link; a create goes through no link")

// parents makes each missing directory of dir under r, outermost first,
// and returns the ones it made. A component that is a link refuses the
// create with an error naming that component, whether its target is in the
// root or not: a create is the one write where the tool picks the place, so
// a place that is not what its name says is refused. Lstat reads each
// component without following it.
func parents(r *os.Root, dir string) ([]string, error) {
	var made []string
	if dir == "." {
		return made, nil
	}
	p := ""
	for _, c := range strings.Split(filepath.Clean(dir), string(filepath.Separator)) {
		p = filepath.Join(p, c)
		fi, err := r.Lstat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := r.Mkdir(p, 0o755); err != nil {
				return made, err
			}
			made = append(made, p)
		case err != nil:
			return made, err
		case fi.Mode()&fs.ModeSymlink != 0:
			return made, &fs.PathError{Op: "create", Path: p, Err: errLinkParent}
		}
	}
	return made, nil
}

// unmake removes the directories parents made, innermost first, so a
// refused create leaves the root listing what it listed before.
func unmake(r *os.Root, made []string) {
	for i := len(made) - 1; i >= 0; i-- {
		r.Remove(made[i]) //nolint:errcheck // best effort: the create's error is the one to report
	}
}

// publish is the step that links the temporary at name, and discard the
// one that removes the temporary; variables so a test can make either fail.
var (
	publish = (*os.Root).Link
	discard = (*os.Root).Remove
)

// LeftError is CreateFile's error when name holds the whole file but its
// temporary could not be removed: Temp, a path under the root, is a second
// name of the same file, which a person may remove; Err is the removal's
// error.
type LeftError struct {
	Temp string
	Err  error
}

func (e *LeftError) Error() string {
	return "written, but its temporary " + e.Temp + " was not removed: " + e.Err.Error()
}

func (e *LeftError) Unwrap() error { return e.Err }
