//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// A named pipe at config.json leaves the Stop hook's summary switch on, and
// the read returns at once: before STORE-READ-REGULAR1 the open waited for a
// writer, and the Stop hook with it, every turn.
func TestSRR1_AFIFOConfigKeepsTheSummaryOnAtOnce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, store.FileConfig)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", path, err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	done, on := make(chan struct{}), false
	go func() { on = summaryOn(root, "k"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("summaryOn had not returned after 2s: the config read waits on the named pipe for a writer")
	}
	if !on {
		t.Error("summaryOn with a named pipe at config.json = false, want the summary on")
	}
}
