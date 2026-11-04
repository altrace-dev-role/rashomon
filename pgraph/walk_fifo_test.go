//go:build unix

package pgraph

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestPW1b_TheWalkNeverOpensAPipe: the walk runs in a hook after every
// reply, so a named pipe called x.go must count by its
// name and never be opened: an open of a pipe with no writer waits forever.
// Break: hash every file the names select, and this times out.
func TestPW1b_TheWalkNeverOpensAPipe(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "x.go"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := WalkDigest(context.Background(), root)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("WalkDigest with a pipe named x.go: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WalkDigest opened a named pipe and waited on it")
	}
}
