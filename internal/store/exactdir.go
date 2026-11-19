package store

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotExactDir is MkdirExact's refusal: a folder on its path is a link or
// not a folder, or, at a level purge judges by exact name, is listed in its
// parent under another case of its name.
var ErrNotExactDir = errors.New("store: a folder under the root is a link, not a folder, or another case of its name")

// MkdirExact makes the folder root/segs[0]/.../segs[len(segs)-1] and returns
// its path; the first exact segments are the levels purge judges by exact
// name. Stub until its GREEN: os.MkdirAll, which follows a link and, on a
// disk that folds case, takes a folder made earlier under another case.
func MkdirExact(root string, exact int, segs ...string) (string, error) {
	dir := filepath.Join(append([]string{root}, segs...)...)
	return dir, os.MkdirAll(dir, dirMode)
}
