package acceptance

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// ptsName unlocks the slave of an open /dev/ptmx and returns its path:
// unlockpt(3) and ptsname(3), which on linux are two ioctls on the master.
// Linux needs no grantpt: devpts creates the slave with the right owner.
func ptsName(master *os.File) (string, error) {
	fd := master.Fd()
	var unlock int32
	if err := ioctl(fd, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		return "", err
	}
	var n uint32
	if err := ioctl(fd, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}
	return "/dev/pts/" + strconv.FormatUint(uint64(n), 10), nil
}
