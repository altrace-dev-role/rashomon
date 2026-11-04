package pgraph

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestPW1b_TheWalkCountsWhatASyncCanRead (card PW1b-ON-230): the hook skips
// a load when WalkDigest is the one the last sync wrote, so the digest must
// move with every file a sync of the root reads (a .go file that is not a
// test, any module's go.mod or go.sum, a nested module's source) and stay
// put for the rest (a test file, a README, testdata, a dot or underscore
// directory). Break: skip a nested module, count test files, or leave go.sum
// out, and this fails.
func TestPW1b_TheWalkCountsWhatASyncCanRead(t *testing.T) {
	root := t.TempDir()
	put := func(name, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("go.mod", "module example.com/m\n")
	put("a/a.go", "package a\n")
	put("sub/go.mod", "module example.com/s\n")
	put("sub/s.go", "package s\n")
	digest := func() string {
		t.Helper()
		d, err := WalkDigest(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		if len(d) != 64 {
			t.Fatalf("WalkDigest = %q, want 64 hex digits", d)
		}
		return d
	}
	last := digest()
	for _, c := range []struct {
		name, body string
		moves      bool
	}{
		{"a/a.go", "package a\n\nfunc F() {}\n", true},
		{"a/new.go", "package a\n", true},
		{"go.sum", "example.com/x v1.0.0 h1:x\n", true},
		{"sub/s.go", "package s\n\nfunc S() {}\n", true},
		{"sub/go.mod", "module example.com/s\n\ngo 1.25.0\n", true},
		{"a/a_test.go", "package a\n", false},
		{"README.md", "notes\n", false},
		{"a/testdata/t.go", "package t\n", false},
		{".hidden/h.go", "package h\n", false},
		{"_tool/x.go", "package x\n", false},
		{"vendor/v/v.go", "package v\n", false},
	} {
		put(c.name, c.body)
		now := digest()
		if moved := now != last; moved != c.moves {
			t.Errorf("writing %s: digest moved %v, want %v", c.name, moved, c.moves)
		}
		last = now
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := WalkDigest(ctx, root); err == nil {
		t.Error("a walk whose context has ended returned a digest")
	}
}
