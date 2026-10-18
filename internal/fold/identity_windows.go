//go:build windows

package fold

import (
	"os"
	"syscall"
)

// identity reads path's volume serial number and file index, Windows' own
// stand-in for a unix device and inode (the same pair os/types_windows.go's
// fileStat uses for os.SameFile): a full replace -- the file at this path
// removed and a different one written in its place -- gives the new file a
// different file index, where a plain append, and an in-place rewrite that
// keeps writing through the same open file (store.rewrite, what forget and
// eviction actually do), never change either number. A missing file reports
// exists=false rather than an error.
func identity(path string) (dev, inode uint64, exists bool, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	defer f.Close() //nolint:errcheck // read-only

	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return 0, 0, false, err
	}
	dev = uint64(info.VolumeSerialNumber)
	inode = uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	return dev, inode, true, nil
}
