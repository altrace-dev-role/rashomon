//go:build unix

package index

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// TestWire_ANamedPipeConfigurationIsRefusedAtOnce (STORE-READ-REGULAR1):
// a named pipe at the configuration file, with no
// writer, refuses Python under that root at once, the error naming the
// file with regfile.ErrNotRegular kept; Go under the same root is refused
// nothing. A configuration step that opened the pipe to wait for a writer
// would hold the hook every turn, so the Claim must return within 2s.
func TestWire_ANamedPipeConfigurationIsRefusedAtOnce(t *testing.T) {
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
	reg := Wire()
	var (
		r   lang.Resolver
		ok  bool
		err error
	)
	done := make(chan struct{})
	go func() { r, ok, err = reg.Claim(root, "inv/core/calc.py"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Claim had not returned after 2s: the Python step waits on the named pipe for a writer")
	}
	if !ok || r != nil || !errors.Is(err, regfile.ErrNotRegular) || !strings.Contains(err.Error(), ConfigFile) {
		t.Errorf("Claim of a .py path = %v, %v, %v; want nil and an error naming %s, ErrNotRegular kept", r, ok, err, ConfigFile)
	}
	g, ok, err := reg.Claim(root, "a/a.go")
	if !ok || err != nil {
		t.Fatalf("Claim of a .go path = %v, %v, %v; want the Go resolver", g, ok, err)
	}
	if got, want := g.OverlayCarried(), (graph.Carried{graph.Declares}); !slices.Equal(got, want) {
		t.Errorf("OverlayCarried() = %v, want %v", got, want)
	}
}

// TestWire_ANamedPipeGoModIsRefusedAtOnce: a named pipe at the root's
// go.mod, with no writer, refuses Go under that root at once, the error
// naming go.mod with regfile.ErrNotRegular kept, as Python's step refuses
// its configuration. The read of go.mod is bounded in bytes, not in time:
// only an open that neither waits nor reads anything but a regular file
// keeps a named pipe from holding the hook, so the Claim must return
// within 2s.
func TestWire_ANamedPipeGoModIsRefusedAtOnce(t *testing.T) {
	root := t.TempDir()
	at := filepath.Join(root, "go.mod")
	if err := syscall.Mkfifo(at, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", at, err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(at, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close() //nolint:errcheck // only wakes a waiting reader
		}
	})
	reg := Wire()
	var (
		r   lang.Resolver
		ok  bool
		err error
	)
	done := make(chan struct{})
	go func() { r, ok, err = reg.Claim(root, "a/a.go"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Claim had not returned after 2s: the Go step waits on the named pipe at go.mod for a writer")
	}
	if !ok || r != nil || !errors.Is(err, regfile.ErrNotRegular) || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("Claim of a .go path = %v, %v, %v; want nil and an error naming go.mod, ErrNotRegular kept", r, ok, err)
	}
}
