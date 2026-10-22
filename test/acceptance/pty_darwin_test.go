package acceptance

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

// ptsName grants and unlocks the slave of an open /dev/ptmx and returns its
// path: grantpt(3), unlockpt(3) and ptsname(3), which on darwin are three
// ioctls on the master.
func ptsName(master *os.File) (string, error) {
	fd := master.Fd()
	if err := ioctl(fd, syscall.TIOCPTYGRANT, 0); err != nil {
		return "", err
	}
	if err := ioctl(fd, syscall.TIOCPTYUNLK, 0); err != nil {
		return "", err
	}
	buf := make([]byte, 128) // the size TIOCPTYGNAME's encoding declares
	if err := ioctl(fd, syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&buf[0]))); err != nil {
		return "", err
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf), nil
}
