package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// conflictBaseOfSize writes a base of exactly size bytes at a fresh path: a
// head at conflictBaseVersion, then a body that decodes, padded with spaces,
// which json.Unmarshal takes after a value. Any prefix of it past the body's
// braces reads back, so only a reader's cap can refuse it.
func conflictBaseOfSize(t *testing.T, size int) string {
	t.Helper()
	pre := `{"v":` + strconv.Itoa(conflictBaseVersion) + "}\n{}"
	if size < len(pre) {
		t.Fatalf("size %d is under the %d bytes of a base", size, len(pre))
	}
	path := filepath.Join(t.TempDir(), "base")
	if err := os.WriteFile(path, []byte(pre+strings.Repeat(" ", size-len(pre))), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestConflictBase_ABaseOverItsCapIsRefused: the post's reader takes a base
// of exactly maxConflictBaseBytes and refuses one a byte longer, though the
// bytes it read decode: the refusal is the cap's, not the parse's. Break:
// drop the length check after the limited read, and the longer base reads.
func TestConflictBase_ABaseOverItsCapIsRefused(t *testing.T) {
	for _, c := range []struct {
		size int
		want bool
	}{{maxConflictBaseBytes, true}, {maxConflictBaseBytes + 1, false}} {
		if _, ok := readConflictBase(conflictBaseOfSize(t, c.size)); ok != c.want {
			t.Errorf("readConflictBase of %d bytes: ok %v, want %v (cap %d)", c.size, ok, c.want, maxConflictBaseBytes)
		}
	}
}

// TestConflictBase_AHeadWithNoNewlineInItsCapIsRefused: the prompt's head
// reader takes a head line whose newline is the last of its
// maxConflictBaseHead bytes and refuses one a byte longer, though the bytes
// it read decode: a head it cannot see end is not read. Break: drop the
// newline check, and the longer head reads.
func TestConflictBase_AHeadWithNoNewlineInItsCapIsRefused(t *testing.T) {
	head := `{"v":` + strconv.Itoa(conflictBaseVersion) + `}`
	for _, c := range []struct {
		line int
		want bool
	}{{maxConflictBaseHead - 1, true}, {maxConflictBaseHead, false}} {
		path := filepath.Join(t.TempDir(), "base")
		data := head + strings.Repeat(" ", c.line-len(head)) + "\n{}\n"
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := readConflictBaseHead(path); ok != c.want {
			t.Errorf("readConflictBaseHead of a %d-byte head line: ok %v, want %v (cap %d)", c.line, ok, c.want, maxConflictBaseHead)
		}
	}
}
