//go:build unix

package knowledge

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// readSafeReturnsInside makes path a named pipe with no writer, runs call
// on it, and fails t unless the call returns inside two seconds. Its
// cleanup opens the writer end, so a call still waiting in its open
// returns and ends. A test file is not importable, so the same helper is
// in internal/graph, internal/hook, internal/graph/lang/gopg and
// pgraph/cmd/pgraph; pgraph's copy also names the frame it waits in.
func readSafeReturnsInside(t *testing.T, path string, call func(path string)) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})
	done := make(chan struct{})
	go func() { defer close(done); call(path) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: still waiting after 2s on a named pipe with no writer", filepath.Base(path))
	}
}

// TestRS1a_R15_ReadDirNamedPipeReturnsAtOnce: a knowledge file that is a
// named pipe with no writer must not hold the index build.
func TestRS1a_R15_ReadDirNamedPipeReturnsAtOnce(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	readSafeReturnsInside(t, filepath.Join(dir, "a.json"), func(string) { ReadDir(root) })
}
