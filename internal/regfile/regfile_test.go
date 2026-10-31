//go:build unix

package regfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Open refuses at once what is not a regular file, opens what is, and leaves
// a missing path as the open's own error. The named pipe has no writer, so an
// open that waited for one would hold the row past its watchdog.
func TestSRR1_OpenRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path string
		want       error // nil: opens
	}{
		{"named pipe", pipe, ErrNotRegular},
		{"directory", dir, ErrNotRegular},
		{"regular file", file, nil},
		{"missing", filepath.Join(dir, "none"), fs.ErrNotExist},
	} {
		var f *os.File
		var err error
		done := make(chan struct{})
		go func() { f, err = Open(c.path); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: Open had not returned after 2s", c.name)
		}
		if f != nil {
			f.Close() //nolint:errcheck // read-only
		}
		switch {
		case c.want == nil && err != nil:
			t.Errorf("%s: Open = %v, want it open", c.name, err)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: Open = %v, want %v", c.name, err, c.want)
		case c.want != nil && f != nil:
			t.Errorf("%s: Open returned a file with error %v", c.name, err)
		}
	}
}
