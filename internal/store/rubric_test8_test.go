package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRubricTEST8_StoreLineAtNAndNPlusOne: a record line of maxLine bytes,
// not counting its newline, is read whole; one byte more is not read, and
// the reader hands its caller a nil line in its place, which every caller
// counts as a damaged line, rather than any part of the record
// (forEachLine).
func TestRubricTEST8_StoreLineAtNAndNPlusOne(t *testing.T) {
	const head, tail = `{"type":"coverage","pad":"`, `"}`
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
		if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var got [][]byte
		if err := eachLine(path, func(l []byte) { got = append(got, l) }); err != nil {
			t.Fatalf("a %d-byte line: %v", tc.n, err)
		}
		switch {
		case len(got) != 1:
			t.Errorf("a %d-byte line: handed over %d lines, want 1", tc.n, len(got))
		case tc.read && len(got[0]) != tc.n:
			t.Errorf("a %d-byte line: read %d bytes, want all %d", tc.n, len(got[0]), tc.n)
		case !tc.read && got[0] != nil:
			t.Errorf("a %d-byte line: read %d bytes of it, want a nil line in its place", tc.n, len(got[0]))
		}
	}
}
