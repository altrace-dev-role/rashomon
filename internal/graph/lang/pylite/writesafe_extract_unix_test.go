//go:build unix

package pylite

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// writeSafeReturnsInside makes path a named pipe with no writer, runs call
// on it, and fails t unless the call returns inside two seconds. Its
// cleanup opens the writer end, so a call still waiting in its open
// returns and ends. The form of the read cards' helper of the same job.
func writeSafeReturnsInside(t *testing.T, path string, call func(path string)) {
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

// TestWS1_J11_ExtractOfANamedPipeReturnsAtOnce: a listed .py that is a named
// pipe with no writer returns at once with an error of the not-regular
// class. Break: open the source by path with a blocking open.
func TestWS1_J11_ExtractOfANamedPipeReturnsAtOnce(t *testing.T) {
	root, r := writeSafeRoot(t)
	var err error
	writeSafeReturnsInside(t, filepath.Join(root, "app", "x.py"), func(string) {
		_, err = r.Extract(root, []string{"app/x.py"}, graph.RevCommit("C1"))
	})
	if !errors.Is(err, regfile.ErrNotRegular) {
		t.Errorf("Extract of a named pipe = %v; want an error of the not-regular class", err)
	}
}
