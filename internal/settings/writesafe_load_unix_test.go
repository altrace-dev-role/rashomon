//go:build unix

package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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

// TestWS1_J3_LoadOfANamedPipeReturnsAtOnce: a settings path that is a named
// pipe with no writer returns at once with an error of the not-regular
// class naming the path; a hook reaches Load (internal/hook/coverage.go),
// and a read that waited would hold every turn. Break: open by path with a
// blocking open.
func TestWS1_J3_LoadOfANamedPipeReturnsAtOnce(t *testing.T) {
	var err error
	p := filepath.Join(t.TempDir(), "settings.json")
	writeSafeReturnsInside(t, p, func(p string) { _, err = Load(p) })
	if !errors.Is(err, regfile.ErrNotRegular) || !strings.Contains(fmtErr(err), p) {
		t.Errorf("Load of a named pipe = %v; want an error of the not-regular class naming %s", err, p)
	}
}

// fmtErr is err's text, or "" for nil.
func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
