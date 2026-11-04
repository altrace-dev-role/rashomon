//go:build unix

package platform

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// fifoAt replaces p with a named pipe no one writes, and wakes any reader
// left on it when the test ends.
func fifoAt(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
}

// TestPLW1y_ANamedPipeNeverHoldsTheRead (M5, STORE-READ-REGULAR1): a named
// pipe where a go.mod, the log, the digests, a written Go file or a
// package's directory should be returns at once, read as no package, an
// unreadable log, stale facts, or no test file; none waits for a writer that
// never comes. Break: open any of the five with os.Open, and the read waits
// until the verb's own timeout.
func TestPLW1y_ANamedPipeNeverHoldsTheRead(t *testing.T) {
	for _, c := range []struct {
		name string
		pipe func(root string) string
		want func(verify.PlatformRead) bool
	}{
		{"go.mod", func(r string) string { return filepath.Join(r, "go.mod") },
			func(p verify.PlatformRead) bool { return len(p.Families) == 0 }},
		{"the log", func(r string) string { return LogPath(r) },
			func(p verify.PlatformRead) bool { return p.Log == verify.LogUnreadable }},
		{"the digests", func(r string) string { return digestsPath(r) },
			func(p verify.PlatformRead) bool {
				return len(p.Families) == 1 && p.Families[0].Facts == verify.FactsStale
			}},
		{"a written file", func(r string) string { return filepath.Join(r, "a", "a.go") },
			func(p verify.PlatformRead) bool {
				return len(p.Families) == 1 && p.Families[0].Facts == verify.FactsStale
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, lp := synced(t)
			fifoAt(t, c.pipe(root))
			done := make(chan verify.PlatformRead, 1)
			go func() { done <- Read(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp}) }()
			select {
			case got := <-done:
				if !c.want(got) {
					t.Errorf("a pipe at %s: %+v", c.name, got)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("a pipe at %s: Read had not returned after 2s", c.name)
			}
		})
	}
	// A package's directory: Read's go.mod probe already finds no package
	// under a pipe, so the listing is reached only in a race; hasTests is
	// held to it directly.
	t.Run("a package's directory", func(t *testing.T) {
		root, _ := synced(t)
		fifoAt(t, filepath.Join(root, "c"))
		done := make(chan bool, 1)
		go func() { done <- hasTests(root, "c") }()
		select {
		case got := <-done:
			if got {
				t.Error("a pipe at a package's directory reads as holding a test file")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("a pipe at a package's directory: hasTests had not returned after 2s")
		}
	})
}
