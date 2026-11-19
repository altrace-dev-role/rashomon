//go:build unix

package index

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// TestReadConfig_ANamedPipeReadsUnreadableAtOnce: a named pipe at
// .rashomon/graph.json ends ReadConfig at once with an error matching
// regfile.ErrNotRegular: the Stop reads the file to name the unit a write is
// in, and an open that waited for a writer would hold the hook.
func TestReadConfig_ANamedPipeReadsUnreadableAtOnce(t *testing.T) {
	root := t.TempDir()
	at := filepath.Join(root, filepath.FromSlash(ConfigFile))
	if err := os.MkdirAll(filepath.Dir(at), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(at, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", at, err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(at, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	var err error
	done := make(chan struct{})
	go func() { _, err = ReadConfig(root); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadConfig had not returned after 2s: it waits on the named pipe for a writer")
	}
	if !errors.Is(err, regfile.ErrNotRegular) {
		t.Errorf("ReadConfig on a named pipe = %v, want ErrNotRegular", err)
	}
}
