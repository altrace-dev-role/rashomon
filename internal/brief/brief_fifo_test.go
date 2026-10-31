//go:build unix

package brief

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBR1n_APipeNamedGoModDoesNotHangTheBrief: a go.mod that is a named pipe
// is no manifest, and Build returns at once instead of waiting in the open
// for a writer that never comes (the SessionStart hook and the brief verb
// both run Build). module on the pipe, were it reached, returns "" at once
// too. Break: drop the regular-file check in the build line, or open without
// O_NONBLOCK, and the call blocks until the test times out.
func TestBR1n_APipeNamedGoModDoesNotHangTheBrief(t *testing.T) {
	root := project(t, map[string]string{"a/x.go": "package a\n"})
	fifo := filepath.Join(root, "go.mod")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no named pipes here: %v", err)
	}
	// A module line sits in the pipe, held by a reader of the test's own and
	// with no writer left, so that a read of it, were one made, would find
	// the line and then reach the end.
	hold, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Skipf("no reader for a named pipe here: %v", err)
	}
	defer hold.Close() //nolint:errcheck // test
	w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Skipf("no writer for a named pipe here: %v", err)
	}
	if _, err := w.WriteString("module evil\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan Brief, 1)
	go func() { done <- Build(root, "", Pointer{}, nil) }()
	select {
	case b := <-done:
		if len(b.Manifests) != 0 || b.Module != "" || !strings.HasPrefix(fact(t, b, KindBuild).Text, "no manifest") ||
			fact(t, b, KindPackages).Text != "not read (no go.mod)" {
			t.Errorf("a pipe named go.mod: manifests %q, module %q, build %q, packages %q; want none read",
				b.Manifests, b.Module, fact(t, b, KindBuild).Text, fact(t, b, KindPackages).Text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Build is blocked on a pipe named go.mod")
	}
	read := make(chan string, 1)
	go func() { read <- module(fifo) }()
	select {
	case m := <-read:
		if m != "" {
			t.Errorf("module of a pipe = %q, want none", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("module is blocked on a pipe")
	}
}
