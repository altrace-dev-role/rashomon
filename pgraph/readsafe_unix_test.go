//go:build unix

package pgraph

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// readSafeModule is a module with no requirements and one package file,
// and no go.sum: Load reads go.mod and a.go, and adds go.sum when present.
func readSafeModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{"go.mod": "module example.com/rs\n\ngo 1.21\n", "a.go": "package rs\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// readSafeReturnsInsideAt is the readSafeReturnsInside of
// internal/graph/readsafe_unix_test.go (copies in internal/hook,
// internal/graph/knowledge, internal/graph/lang/gopg and pgraph/cmd/pgraph;
// a test file is not importable) with one addition for a call that runs a
// go list child: at the deadline it dumps every goroutine and says whether
// frame is among them, so a wait inside the child is told from a wait in
// frame's own open.
func readSafeReturnsInsideAt(t *testing.T, path, frame string, call func(path string)) {
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
		buf := make([]byte, 1<<20)
		stacks := string(buf[:runtime.Stack(buf, true)])
		if !strings.Contains(stacks, frame) {
			t.Fatalf("%s: still waiting after 2s, but no goroutine is in %s: the wait is elsewhere\n%s", filepath.Base(path), frame, stacks)
		}
		t.Fatalf("%s: still waiting after 2s in %s on a named pipe with no writer", filepath.Base(path), frame)
	}
}

// TestRS1a_R18_LoadGoSumNamedPipeReturnsAtOnce: a go.sum that is a named
// pipe with no writer must not hold a sync. go list does not read go.sum
// in a module with no requirements, so the wait, if any, is pgraph's own
// hash of it (ingest.go add -> hashFile). The load must also refuse the
// pipe, not hash it: a read of a pipe with no writer ends at once with no
// bytes, so without the refusal the pipe would enter the program as an
// empty go.sum.
func TestRS1a_R18_LoadGoSumNamedPipeReturnsAtOnce(t *testing.T) {
	root := readSafeModule(t)
	if _, err := Load(t.Context(), root, Config{}); err != nil {
		t.Fatalf("the module without go.sum does not load: %v", err)
	}
	var err error // written by call, read after readSafeReturnsInsideAt saw it return
	readSafeReturnsInsideAt(t, filepath.Join(root, "go.sum"), "pgraph.hashFile(", func(string) {
		_, err = Load(t.Context(), root, Config{})
	})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Load with a named pipe at go.sum: %v, want the not-a-regular-file refusal", err)
	}
}

// TestRS1a_R19_LoadGoSumOverTheCapIsRefused: a go.sum one byte over
// MaxSourceFileBytes is refused by the source cap, not hashed.
// TestPL1_CapsAndFailClosed holds the same cap through a .go file.
func TestRS1a_R19_LoadGoSumOverTheCapIsRefused(t *testing.T) {
	root := readSafeModule(t)
	f, err := os.Create(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxSourceFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := Load(t.Context(), root, Config{}); err == nil || !strings.Contains(err.Error(), "source cap") {
		t.Fatalf("Load with a %d byte go.sum: %v, want the source cap refusal", MaxSourceFileBytes+1, err)
	}
}
