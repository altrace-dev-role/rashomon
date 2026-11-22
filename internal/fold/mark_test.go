package fold

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMarkOf_IsTheLengthAndTheHexDigest: a mark is the line's length and the
// SHA-256 of its bytes in lowercase hex, checked against the digests FIPS
// 180-2 publishes for "abc" and for no bytes.
func TestMarkOf_IsTheLengthAndTheHexDigest(t *testing.T) {
	for line, want := range map[string]LineMark{
		"abc": {Len: 3, SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		"":    {Len: 0, SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	} {
		if got := MarkOf([]byte(line)); *got != want {
			t.Errorf("MarkOf(%q) = %+v, want %+v", line, *got, want)
		}
	}
}

// TestCursorTailValid: the line that ends at the cursor must be the marked
// one, its '\r' counted, starting the file or after a line end; a zero
// offset is valid with no mark and no file. Every other case is refused with
// no error: no mark, another line of the same length, a mark of the line's
// own tail, a length past the offset or below zero, no line end at the
// cursor, a file shorter than the size seen or than the offset, and no file.
func TestCursorTailValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "records.ndjson")
	const body = "first line\nsecond\r\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	end := int64(len(body))
	at := func(off int64) Cursor { return Cursor{Offset: off, SizeSeen: end} }
	second := MarkOf([]byte("second\r"))
	for _, tc := range []struct {
		name string
		path string
		c    Cursor
		m    *LineMark
		want bool
	}{
		{"the marked last line, its \\r counted", path, at(end), second, true},
		{"the first line, at its own end", path, at(11), MarkOf([]byte("first line")), true},
		{"offset 0, no mark, no file", filepath.Join(dir, "absent"), Cursor{}, nil, true},
		{"no mark at a positive offset", path, at(end), nil, false},
		{"another line of the same length", path, at(end), MarkOf([]byte("SECOND\r")), false},
		{"the line without its \\r", path, at(end), MarkOf([]byte("second")), false},
		{"a mark of the line's own tail", path, at(end), MarkOf([]byte("econd\r")), false},
		{"a length past the offset", path, at(end), &LineMark{Len: end, SHA256: second.SHA256}, false},
		{"a length below zero", path, at(end), &LineMark{Len: -1, SHA256: second.SHA256}, false},
		{"no line end at the cursor", path, at(end - 1), MarkOf([]byte("second")), false},
		{"a file shorter than the size seen", path, Cursor{Offset: end, SizeSeen: end + 1}, second, false},
		{"an offset past the file", path, Cursor{Offset: end + 1, SizeSeen: end}, MarkOf([]byte("second\r\n")), false},
		{"no file", filepath.Join(dir, "absent"), at(end), second, false},
	} {
		got, err := CursorTailValid(tc.path, tc.c, tc.m)
		if got != tc.want || err != nil {
			t.Errorf("%s: %v, %v; want %v, nil", tc.name, got, err, tc.want)
		}
	}
}
