//go:build unix

package pylite

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// TestLeftOut_ANamedPipeOrALinkNamedPyvenvCfgIsNotAnEnvironment: LeftOut
// stats pyvenv.cfg without following a link and reads no file, as its doc
// says, so for a named pipe of that name it returns at once and names no
// environment, and it names none for a link of that name, to a regular
// file or to nothing; the regular pyvenv.cfg the link points at makes its
// own directory an environment. Unix only: named pipes. Break: follow the
// link (os.Stat), which takes a link to a regular file for an environment;
// refuse only a directory, which takes the pipe and both links for one;
// read the file, which waits on the pipe for a writer.
func TestLeftOut_ANamedPipeOrALinkNamedPyvenvCfgIsNotAnEnvironment(t *testing.T) {
	root := t.TempDir()
	at := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	for _, d := range []string{"venv", "pipe", "link", "dangling"} {
		if err := os.MkdirAll(at(d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(at("venv/pyvenv.cfg"), []byte("home = /usr/bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pipe := at("pipe/pyvenv.cfg")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", pipe, err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	if err := os.Symlink(filepath.Join("..", "venv", "pyvenv.cfg"), at("link/pyvenv.cfg")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(at("link/pyvenv.cfg")); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("link/pyvenv.cfg does not lead to a regular file: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "gone", "pyvenv.cfg"), at("dangling/pyvenv.cfg")); err != nil {
		t.Fatal(err)
	}
	env := lang.LeftEnvironment + " (holds pyvenv.cfg)"
	for _, c := range []struct {
		rel, why string
	}{
		{"venv", env},
		{"pipe", ""},
		{"link", ""},
		{"dangling", ""},
	} {
		var why string
		var ok bool
		done := make(chan struct{})
		go func() { why, ok = Resolver{}.LeftOut(root, c.rel); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("LeftOut(%s) had not returned after 2s: it waits on the named pipe for a writer", c.rel)
		}
		if why != c.why || ok != (c.why != "") {
			t.Errorf("LeftOut(%s) = %q, %v; want %q, %v", c.rel, why, ok, c.why, c.why != "")
		}
	}
}
