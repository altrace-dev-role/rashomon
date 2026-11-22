package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestRunRegionFrom_RawIsTheBytesOnDisk: raw is the region of whole lines as
// the file holds it, a '\r' and an empty line kept, where the lines drop
// both; a torn last line is in neither, and end stops before it. From an
// offset, raw starts there. With no whole line past the offset, raw is empty
// and end is the offset. RunLinesFrom is the same read without raw.
func TestRunRegionFrom_RawIsTheBytesOnDisk(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "sess-region"
	if err := os.MkdirAll(st.RunDir(id), dirMode); err != nil {
		t.Fatal(err)
	}
	const data = "a\r\n\nb\n{\"torn\""
	if err := os.WriteFile(filepath.Join(st.RunDir(id), FileRecords), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		offset int64
		lines  []string
		raw    string
		end    int64
	}{
		{0, []string{"a", "b"}, "a\r\n\nb\n", 6},
		{3, []string{"b"}, "\nb\n", 6},
		{6, nil, "", 6},
	} {
		lines, raw, end, size, err := st.RunRegionFrom(id, FileRecords, tc.offset, -1)
		if err != nil {
			t.Fatalf("offset %d: %v", tc.offset, err)
		}
		var got []string
		for _, l := range lines {
			got = append(got, string(l))
		}
		if !reflect.DeepEqual(got, tc.lines) || string(raw) != tc.raw || end != tc.end || size != int64(len(data)) {
			t.Errorf("offset %d: lines %q, raw %q, end %d, size %d; want %q, %q, %d, %d", tc.offset, got, raw, end, size, tc.lines, tc.raw, tc.end, len(data))
		}
		ll, lend, lsize, err := st.RunLinesFrom(id, FileRecords, tc.offset, -1)
		if err != nil || !reflect.DeepEqual(ll, lines) || lend != end || lsize != size {
			t.Errorf("offset %d: RunLinesFrom is not RunRegionFrom's read: %q, %d, %d, %v", tc.offset, ll, lend, lsize, err)
		}
	}
}
