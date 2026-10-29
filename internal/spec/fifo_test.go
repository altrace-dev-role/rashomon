//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package spec

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestVER41c_OSFSRefusesAFIFO: a FIFO a repository plants at a candidate path
// is refused by OSFS.ReadFile with the non-regular reason, before any open,
// so a hook call never waits for a writer (card 3.1 (c)). Opened for reading,
// a FIFO blocks until a writer comes: if ReadFile has not returned within 2 s
// the row opens one and closes it, so the read ends and the row fails on its
// error. The tag is unix less the systems with no syscall.Mkfifo (aix,
// solaris, illumos); a filesystem that refuses a FIFO skips the row.
func TestVER41c_OSFSRefusesAFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("mkfifo %s: %v", path, err)
	}
	type read struct {
		data []byte
		err  error
	}
	done := make(chan read, 1)
	go func() {
		data, err := OSFS{}.ReadFile(path)
		done <- read{data, err}
	}()
	var got read
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("ReadFile(%s) waited 2 s on a FIFO", path)
		if w, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
			w.Close()
		}
		got = <-done
	}
	if want := "spec: " + path + " is not a regular file"; got.err == nil || got.err.Error() != want || got.data != nil {
		t.Errorf("ReadFile(%s) = %q, %v; want nil and %q", path, got.data, got.err, want)
	}
}
