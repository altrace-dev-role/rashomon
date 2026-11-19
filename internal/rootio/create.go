package rootio

import (
	"errors"
	"io"
	"io/fs"
	"os"
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
// created with perm under the umask. The error names the target, the root's
// path joined with name, on one line.
func CreateFile(r *os.Root, name string, perm fs.FileMode, max int64, write func(io.Writer) error) error {
	return errors.New("rootio: CreateFile is not built yet")
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
