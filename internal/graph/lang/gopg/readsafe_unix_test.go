//go:build unix

package gopg

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// readSafeReturnsInside makes path a named pipe with no writer, runs call
// on it, and fails t unless the call returns inside two seconds. Its
// cleanup opens the writer end, so a call still waiting in its open
// returns and ends. A test file is not importable, so the same helper is
// in internal/graph, internal/hook, internal/graph/knowledge and
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

// TestRS1a_R16_ReadModulePathNamedPipeReturnsAtOnce: a go.mod that is a
// named pipe with no writer must not hold the hook's overlay or the index
// build, the two callers of ReadModulePath.
func TestRS1a_R16_ReadModulePathNamedPipeReturnsAtOnce(t *testing.T) {
	root := t.TempDir()
	readSafeReturnsInside(t, filepath.Join(root, "go.mod"), func(string) { ReadModulePath(root) })
}

// TestRS1a_R17_ReadModulePathOverTheCapReadsCompleteLines: a go.mod one
// byte over MaxGoModBytes is read through the cap, complete lines only, so
// its module line still names the module; over the cap is not a refusal.
func TestRS1a_R17_ReadModulePathOverTheCapReadsCompleteLines(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("module example.com/m\n")
	for b.Len() <= MaxGoModBytes {
		b.WriteString("// a comment line that pads go.mod past its read cap\n")
	}
	mod := b.String()[:MaxGoModBytes+1]
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadModulePath(root); got != "example.com/m" || err != nil {
		t.Fatalf("ReadModulePath of a %d byte go.mod = %q, %v; want example.com/m and no error", len(mod), got, err)
	}
}
