package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain points CODEX_HOME at a directory that does not exist before any
// test runs. status reads Codex's directory, and a test in this package that
// reached it without this would read the real ~/.codex of whoever ran the
// suite. A missing directory is also the arrangement every test here was
// written against: a machine with no Codex on it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rashomon-codex-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex home:", err)
		os.Exit(1)
	}
	os.Setenv("CODEX_HOME", filepath.Join(dir, "absent"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
