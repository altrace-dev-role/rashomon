package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestSC1_SteerDisclosureMarkerRoundTrip: the steer-disclosure marker holds
// one non-negative decimal integer, the highest steer disclosure consented
// to. It reads back what was written. Absent, malformed or unreadable
// content reads 0 with an error saying why, and 0 is the value that
// withholds. The writer refuses a negative value and leaves the marker as
// it was. Neither the reader nor the writer goes through Open, so a root
// gains no install identity by being asked what it may say.
func TestSC1_SteerDisclosureMarkerRoundTrip(t *testing.T) {
	root := t.TempDir()
	if n, err := SteerDisclosureMarker(root); n != 0 || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent marker = (%d, %v), want (0, fs.ErrNotExist)", n, err)
	}
	if err := WriteSteerDisclosureMarker(root, 2); err != nil {
		t.Fatalf("WriteSteerDisclosureMarker(2): %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, FileSteerDisclosure))
	if err != nil || string(b) != "2\n" {
		t.Fatalf("the marker holds %q (%v), want \"2\\n\"", b, err)
	}
	if n, err := SteerDisclosureMarker(root); n != 2 || err != nil {
		t.Errorf("after writing 2 the marker reads (%d, %v), want (2, nil)", n, err)
	}

	if err := WriteSteerDisclosureMarker(root, -1); err == nil {
		t.Errorf("WriteSteerDisclosureMarker(-1) succeeded, want a refusal")
	}
	if n, err := SteerDisclosureMarker(root); n != 2 || err != nil {
		t.Errorf("a refused write changed the marker: (%d, %v), want (2, nil)", n, err)
	}

	for _, c := range []struct {
		content string
		want    int
		wantErr error
	}{
		{" 2\n", 2, nil},
		{"0\n", 0, nil},
		{"02\n", 2, nil},
		{"v5\n", 0, ErrSteerDisclosureWord},
		{"-1\n", 0, ErrSteerDisclosureWord},
		{"+2\n", 0, ErrSteerDisclosureWord},
		{"2 3\n", 0, ErrSteerDisclosureWord},
		{"", 0, ErrSteerDisclosureWord},
		{"2.0\n", 0, ErrSteerDisclosureWord},
		{"99999999999999999999\n", 0, ErrSteerDisclosureWord},
	} {
		if err := os.WriteFile(filepath.Join(root, FileSteerDisclosure), []byte(c.content), 0o600); err != nil {
			t.Fatal(err)
		}
		n, err := SteerDisclosureMarker(root)
		if n != c.want || (c.wantErr == nil) != (err == nil) || c.wantErr != nil && !errors.Is(err, c.wantErr) {
			t.Errorf("marker %q reads (%d, %v), want (%d, %v)", c.content, n, err, c.want, c.wantErr)
		}
	}

	if err := RemoveSteerDisclosureMarker(root); err != nil {
		t.Fatalf("RemoveSteerDisclosureMarker: %v", err)
	}
	if err := RemoveSteerDisclosureMarker(root); err != nil {
		t.Errorf("RemoveSteerDisclosureMarker with no marker: %v, want none", err)
	}
	if _, err := SteerDisclosureMarker(root); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after remove the marker reads %v, want fs.ErrNotExist", err)
	}

	absent := filepath.Join(t.TempDir(), "never-recorded")
	if n, _ := SteerDisclosureMarker(absent); n != 0 {
		t.Errorf("an absent root reads %d, want 0", n)
	}
	if _, err := os.Stat(absent); err == nil {
		t.Errorf("SteerDisclosureMarker created %s merely by being asked", absent)
	}
	fresh := t.TempDir()
	if err := WriteSteerDisclosureMarker(fresh, 3); err != nil {
		t.Fatalf("WriteSteerDisclosureMarker: %v", err)
	}
	entries, err := os.ReadDir(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != FileSteerDisclosure {
		t.Errorf("a root holding only the marker holds %v, want the marker alone", entries)
	}
}
