//go:build unix

package recap

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fifoAt makes a named pipe at path; its cleanup wakes a read still waiting
// in the pipe's open (internal/store's regular_test.go has the twin).
func fifoAt(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", path, err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
}

// returnsWithin fails the test if read has not returned within d. read must
// not call t.
func returnsWithin(t *testing.T, d time.Duration, read func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { read(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("the read had not returned after %v: it waits on the named pipe for a writer", d)
	}
}

// stateOfSize writes a recap.json that parses and is exactly n bytes long.
func stateOfSize(t *testing.T, path string, n int) {
	t.Helper()
	head, tail := `{"spoken":{"s":"`, `"}}`
	b := head + strings.Repeat("a", n-len(head)-len(tail)) + tail
	if err := os.WriteFile(path, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A named pipe at recap.json returns at once with an error, which every
// caller takes as a fresh state; the writers then replace it.
func TestSRR1_AFIFOStateFileReadsAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	fifoAt(t, path)
	var err error
	returnsWithin(t, 2*time.Second, func() { _, err = readState(path) })
	if err == nil {
		t.Error("readState on a named pipe read as a state; want an error")
	}
}

// A recap.json one byte over its cap is an error, not a read of it whole.
func TestSRR1_AnOverCapStateFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	stateOfSize(t, path, maxStateBytes+1)
	if _, err := readState(path); err == nil {
		t.Error("readState read a state over its cap; want an error")
	}
}
