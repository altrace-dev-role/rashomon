package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPaused_AbsentRootIsNotPausedAndCreatesNothing is the store-level half of
// H-81: asking whether an unpaused, never-touched root is paused must not
// bring the root into existence.
func TestPaused_AbsentRootIsNotPausedAndCreatesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist-yet")

	paused, since, err := Paused(root)
	if err != nil {
		t.Fatalf("Paused: %v", err)
	}
	if paused {
		t.Errorf("a root nobody touched reads as paused")
	}
	if !since.IsZero() {
		t.Errorf("since is %v, want zero", since)
	}
	if _, err := os.Stat(root); err == nil {
		t.Errorf("Paused created %s merely by being asked about it", root)
	}
}

// TestPause_IsIdempotentAndKeepsTheFirstInstant: pausing twice must not push
// the recorded start forward, or a report built from it would understate how
// long the window actually was.
func TestPause_IsIdempotentAndKeepsTheFirstInstant(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	first := time.Unix(1_700_000_000, 0)
	second := first.Add(time.Hour)

	since1, already1, err := Pause(root, first)
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	if already1 {
		t.Errorf("first pause reports alreadyPaused")
	}
	if !since1.Equal(first) {
		t.Errorf("since is %v, want %v", since1, first)
	}

	since2, already2, err := Pause(root, second)
	if err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if !already2 {
		t.Errorf("second pause does not report alreadyPaused")
	}
	if !since2.Equal(first) {
		t.Errorf("second pause's since is %v, want the original %v", since2, first)
	}

	paused, since, err := Paused(root)
	if err != nil || !paused {
		t.Fatalf("Paused after Pause: paused=%v err=%v", paused, err)
	}
	if !since.Equal(first) {
		t.Errorf("Paused reports since %v, want %v", since, first)
	}
}

// TestPause_CreatesOnlyTheMarkerNotAnInstallIdentity: pausing a machine that
// has never recorded must not mint install.json or install.key -- the two
// files that make Open's identity real.
func TestPause_CreatesOnlyTheMarkerNotAnInstallIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	if _, _, err := Pause(root, time.Now()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	for _, name := range []string{fileInstallMeta, fileInstallKey, dirRuns} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("Pause created %s, which belongs to an opened store", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, fileRecordingPaused)); err != nil {
		t.Errorf("Pause did not create its own marker: %v", err)
	}
}

// TestResume_ClosesTheWindowAndIsIdempotentWhenNotPaused.
func TestResume_ClosesTheWindowAndIsIdempotentWhenNotPaused(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")

	wasPaused, since, err := Resume(root, time.Now())
	if err != nil {
		t.Fatalf("Resume on an untouched root: %v", err)
	}
	if wasPaused {
		t.Errorf("Resume reports wasPaused on a root that was never paused")
	}
	if !since.IsZero() {
		t.Errorf("since is %v, want zero", since)
	}

	start := time.Unix(1_700_000_000, 0)
	if _, _, err := Pause(root, start); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	end := start.Add(10 * time.Minute)
	wasPaused, since, err = Resume(root, end)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !wasPaused {
		t.Fatalf("Resume does not report wasPaused after a real Pause")
	}
	if !since.Equal(start) {
		t.Errorf("Resume's since is %v, want %v", since, start)
	}

	paused, _, err := Paused(root)
	if err != nil {
		t.Fatalf("Paused after Resume: %v", err)
	}
	if paused {
		t.Errorf("still reads as paused after Resume")
	}

	// A second resume is a satisfied request, not an error, matching detach.
	wasPaused, since, err = Resume(root, end.Add(time.Minute))
	if err != nil {
		t.Fatalf("second Resume: %v", err)
	}
	if wasPaused || !since.IsZero() {
		t.Errorf("second Resume reports wasPaused=%v since=%v, want false/zero", wasPaused, since)
	}
}
