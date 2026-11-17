//go:build unix

package baseline

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// fifoAt replaces p with a named pipe no one writes, and wakes any reader
// left on it when the test ends (internal/platform's form).
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

// within runs f on a goroutine of its own and fails the test, naming what
// waited, if f has not returned after 5 s: a read held by a pipe is a named
// FAIL at the deadline, never the package's timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); f() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s had not returned after 5s", what)
	}
}

// TestRepoID_ANamedPipeGivesNoID: a named pipe at
// the worktree's .git pointer, at its commondir or at the main config
// returns at once as no id: RepoIDErr's error matches regfile.ErrNotRegular
// and RepoID is "", ephemeral, never the path form. Nothing writes the
// pipe, so a read that opened it would wait for a writer that never comes,
// on every hook that reads the id. The control, first, on the same tree
// with the three files regular, reads the origin. Break: read any of the
// three with os.ReadFile, and that row fails at the 5 s deadline.
func TestRepoID_ANamedPipeGivesNoID(t *testing.T) {
	for _, name := range []string{"pointer", "commondir", "config"} {
		t.Run(name, func(t *testing.T) {
			wt, files := gitTree(t)
			if rid, eph := RepoID(wt); rid != treeOrigin || eph {
				t.Fatalf("control, regular files: RepoID = %q, %v; want %q, false", rid, eph, treeOrigin)
			}
			fifoAt(t, files[name])
			var rid, plain string
			var eph, plainEph bool
			var err error
			within(t, "RepoID over a pipe at the "+name, func() {
				rid, eph, err = RepoIDErr(wt)
				plain, plainEph = RepoID(wt)
			})
			if rid != "" || !eph || !errors.Is(err, regfile.ErrNotRegular) {
				t.Errorf("a pipe at the %s: RepoIDErr = %q, %v, %v; want no id and an error matching %v", name, rid, eph, err, regfile.ErrNotRegular)
			}
			if plain != "" || !plainEph {
				t.Errorf("a pipe at the %s: RepoID = %q, %v; want no id, never the path form", name, plain, plainEph)
			}
		})
	}
}
