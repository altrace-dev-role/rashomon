//go:build unix

package regfile

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Open refuses a named pipe at once. The pipe has no writer, so an open that
// waited for one would hold the row past its watchdog.
func TestSRR1_OpenRefusesANamedPipeAtOnce(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	var f *os.File
	var err error
	done := make(chan struct{})
	go func() { f, err = Open(pipe); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Open had not returned after 2s")
	}
	if f != nil {
		f.Close() //nolint:errcheck // read-only
		t.Errorf("Open returned a file for a named pipe, error %v", err)
	}
	if !errors.Is(err, ErrNotRegular) {
		t.Errorf("Open = %v, want %v", err, ErrNotRegular)
	}
}
