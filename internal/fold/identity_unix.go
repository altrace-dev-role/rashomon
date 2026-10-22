//go:build unix

package fold

import (
	"fmt"
	"os"
	"syscall"
)

// identity reads path's device and inode, the pair fold.go's Dev/Inode
// fields guard records.ndjson's cursors with: a full replace -- the file
// at this path removed and a different one written in its place -- gives
// the new file a different inode, where a plain append, and an in-place
// rewrite that keeps writing through the same open file (store.rewrite,
// what forget and eviction actually do), never change either number. A
// missing file reports exists=false rather than an error -- "this run has
// no records.ndjson yet" is not a read failure.
func identity(path string) (dev, inode uint64, exists bool, err error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false, fmt.Errorf("fold: %s: FileInfo.Sys() is not *syscall.Stat_t", path)
	}
	return uint64(st.Dev), uint64(st.Ino), true, nil //nolint:unconvert // Dev's width differs by unix flavour
}
