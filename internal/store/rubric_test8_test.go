package store

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRubricTEST8_StoreLineAtNAndNPlusOne: a record line of maxLine bytes,
// counting its newline, is read; one byte more is not, and the read says so
// with bufio.ErrTooLong rather than handing back part of a record. The bound
// counts the newline because the scanner needs room for it: the longest
// record is maxLine-1 bytes.
func TestRubricTEST8_StoreLineAtNAndNPlusOne(t *testing.T) {
	const head, tail = `{"type":"coverage","pad":"`, "\"}\n"
	if maxLine != 16<<20 {
		t.Fatalf("maxLine = %d, want 16 MiB", maxLine)
	}
	for _, tc := range []struct {
		n    int
		read bool
	}{{maxLine, true}, {maxLine + 1, false}} {
		path := filepath.Join(t.TempDir(), "records.ndjson")
		line := head + strings.Repeat("x", tc.n-len(head)-len(tail)) + tail
		if len(line) != tc.n {
			t.Fatalf("built %d bytes, want %d", len(line), tc.n)
		}
		if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
		got := -1
		err := eachLine(path, func(l []byte) { got = len(l) })
		switch {
		case tc.read && (err != nil || got != tc.n-1):
			t.Errorf("a %d-byte line: read %d bytes, %v; want %d and no error", tc.n, got, err, tc.n-1)
		case !tc.read && (!errors.Is(err, bufio.ErrTooLong) || got != -1):
			t.Errorf("a %d-byte line: read %d bytes, %v; want none and bufio.ErrTooLong", tc.n, got, err)
		}
	}
}
