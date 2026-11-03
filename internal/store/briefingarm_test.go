package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCSL1_TheBriefingMarkerIsEmptyAndTakenOnce: ArmBriefing leaves an empty
// file in the session's run directory, mode 0600, and arming twice is one
// arm; TakeBriefingArm takes it once, so of two prompts only one gives the
// briefing, and a session never armed has nothing to take (D-c). Break: write
// anything into the marker, or let a second take succeed, and this fails.
func TestCSL1_TheBriefingMarkerIsEmptyAndTakenOnce(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.TakeBriefingArm("sess-never") {
		t.Error("a session never armed had a marker to take")
	}
	if err := os.MkdirAll(s.RunDir("sess-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.ArmBriefing("sess-1"); err != nil {
			t.Fatalf("ArmBriefing: %v", err)
		}
	}
	fi, err := os.Lstat(filepath.Join(s.RunDir("sess-1"), FileBriefingArmed))
	if err != nil || fi.Size() != 0 || fi.Mode().Perm() != 0o600 || !fi.Mode().IsRegular() {
		t.Fatalf("the marker: %v, %v; want an empty regular file, mode 0600", fi, err)
	}
	if !s.TakeBriefingArm("sess-1") {
		t.Error("the first take of an armed session failed")
	}
	if s.TakeBriefingArm("sess-1") {
		t.Error("a second take succeeded; two prompts would both give the briefing")
	}
}
