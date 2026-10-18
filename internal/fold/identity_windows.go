//go:build windows

package fold

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetFileInformationByHandle = kernel32.NewProc("GetFileInformationByHandle")
)

// byHandleFileInformation mirrors Win32's BY_HANDLE_FILE_INFORMATION, the
// fields GetFileInformationByHandle writes. Only VolumeSerialNumber and the
// two FileIndex halves are read; the rest exists so the struct's layout
// matches the one the call writes into.
type byHandleFileInformation struct {
	FileAttributes     uint32
	CreationTime       syscall.Filetime
	LastAccessTime     syscall.Filetime
	LastWriteTime      syscall.Filetime
	VolumeSerialNumber uint32
	FileSizeHigh       uint32
	FileSizeLow        uint32
	NumberOfLinks      uint32
	FileIndexHigh      uint32
	FileIndexLow       uint32
}

// identity reads path's volume serial number and file index, Windows' own
// stand-in for a unix device and inode (the same pair os/types_windows.go's
// fileStat uses for os.SameFile): a rewrite (forget, eviction) removes the
// old file index and creates a new one at the same path, where a plain
// append never changes either number. A missing file reports exists=false
// rather than an error.
func identity(path string) (dev, inode uint64, exists bool, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	defer f.Close() //nolint:errcheck // read-only

	var info byHandleFileInformation
	r, _, errno := procGetFileInformationByHandle.Call(f.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, 0, false, errno
	}
	dev = uint64(info.VolumeSerialNumber)
	inode = uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	return dev, inode, true, nil
}
