package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotExactDir is MkdirExact's refusal: a folder on its path is a link or
// not a folder, or, at a level purge judges by exact name, is listed in its
// parent under another case of its name.
var ErrNotExactDir = errors.New("store: a folder under the root is a link, not a folder, or another case of its name")

// MkdirExact makes the folder root/segs[0]/.../segs[len(segs)-1] and returns
// its path. Each level under the root must be a real folder: one that Lstat
// reports as a folder, so a link (to a folder or anything else) or a file is
// refused with ErrNotExactDir and nothing is made through it. In the first
// exact levels, the ones purge judges by exact name, the parent's listing
// must also hold the name byte for byte: on a disk that folds case, a folder
// made earlier under another case of the name is found by Lstat but is not
// the folder purge would remove, so it is refused too. Below the exact
// levels any case is taken, as purge takes it. The root itself is not
// checked; it is the store the person chose.
//
// The checks hold for the moment they run. A folder swapped for a link
// between a check and a later write in it is not caught here; closing that
// needs a walk by folder handles that do not follow links.
func MkdirExact(root string, exact int, segs ...string) (string, error) {
	if exact < 0 || exact > len(segs) {
		return "", fmt.Errorf("store: MkdirExact: %d exact levels of %d", exact, len(segs))
	}
	for _, s := range segs {
		if s == "" || s == "." || s == ".." || strings.ContainsRune(s, '/') || strings.ContainsRune(s, filepath.Separator) {
			return "", fmt.Errorf("store: MkdirExact: %q is not one folder name", s)
		}
	}
	dir := root
	for i, s := range segs {
		parent := dir
		dir = filepath.Join(parent, s)
		fi, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(dir, dirMode); err != nil && !errors.Is(err, fs.ErrExist) {
				return "", err
			}
			fi, err = os.Lstat(dir)
		}
		if err != nil {
			return "", err
		}
		rel := filepath.Join(segs[:i+1]...)
		if fi.Mode().Type() != fs.ModeDir {
			return "", fmt.Errorf("%w: %s", ErrNotExactDir, rel)
		}
		if i < exact {
			listed, err := listsName(parent, s)
			if err != nil {
				return "", err
			}
			if !listed {
				return "", fmt.Errorf("%w: %s", ErrNotExactDir, rel)
			}
		}
	}
	return dir, nil
}

// listsName reports whether the folder dir lists an entry named name byte for
// byte. Only the names are read, in the order the disk gives them.
func listsName(dir, name string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	names, err := f.Readdirnames(-1)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}
