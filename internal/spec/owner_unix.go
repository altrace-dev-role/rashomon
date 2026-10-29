//go:build unix

package spec

import (
	"io/fs"
	"os"
	"syscall"
)

// owner is path's owning uid and mode as lstat reads them, so a symlink is
// reported as itself and never as its target; uid -1 when they are unknown.
func owner(path string) (int, fs.FileMode, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return -1, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, fi.Mode(), nil
	}
	return int(st.Uid), fi.Mode(), nil
}
