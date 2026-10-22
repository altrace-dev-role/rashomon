package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TestMarkerUnreadable_TrueOnlyWhenTheMarkerIsAPlainFile: MarkerUnreadable
// must say yes only when the marker itself is a plain file that is there,
// and no for every other shape -- no marker at all, or a directory where
// one would be. It does not itself check that reading fails, since callers
// only ask once Paused's own read already has; checkPaused and
// RecordingState rely on this to fail closed on the marker only when the
// marker is the actual problem, never on an unrelated filesystem fault.
func TestMarkerUnreadable_TrueOnlyWhenTheMarkerIsAPlainFile(t *testing.T) {
	t.Run("no marker", func(t *testing.T) {
		root := t.TempDir()
		if MarkerUnreadable(root) {
			t.Error("MarkerUnreadable is true with no marker at all")
		}
	})

	t.Run("a directory where the marker would be", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, fileRecordingPaused), 0o700); err != nil {
			t.Fatal(err)
		}
		if MarkerUnreadable(root) {
			t.Error("MarkerUnreadable is true for a directory, which Paused fails on for a different reason entirely")
		}
	})

	t.Run("an unreadable plain marker", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("chmod 000 does not stop this user reading the file")
		}
		root := t.TempDir()
		if _, _, err := Pause(root, time.Now()); err != nil {
			t.Fatalf("Pause: %v", err)
		}
		marker := filepath.Join(root, fileRecordingPaused)
		if err := os.Chmod(marker, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(marker, 0o644) }) //nolint:errcheck // TempDir removal follows
		if !MarkerUnreadable(root) {
			t.Error("MarkerUnreadable is false for a plain marker this test just made unreadable")
		}
	})
}

// TestRubricPAUSE9_ResumeRemovesAnUnreadableMarker: a pause marker that exists and
// cannot be read is exactly the state checkPaused fails closed on, which
// makes it sticky -- Resume used to return Paused's raw read error before
// ever reaching the file, leaving the caller unable to clear the one state
// checkPaused treats as a pause.
func TestRubricPAUSE9_ResumeRemovesAnUnreadableMarker(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not stop this user reading the file")
	}
	root := t.TempDir()
	since, _, err := Pause(root, time.Now())
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	marker := filepath.Join(root, fileRecordingPaused)
	if err := os.Chmod(marker, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(marker, 0o644) }) //nolint:errcheck // TempDir removal follows

	wasPaused, closedSince, err := Resume(root, since.Add(time.Minute))
	if err != nil {
		t.Fatalf("Resume: %v, want it to remove the unreadable marker instead", err)
	}
	if !wasPaused {
		t.Error("wasPaused is false, want true: the marker was there")
	}
	if closedSince.IsZero() {
		t.Error("closedSince is zero, want now: the true start was unreadable")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("marker still exists after Resume: %v", err)
	}
	if paused, _, err := Paused(root); err != nil || paused {
		t.Errorf("Paused after Resume = (%v, %v), want (false, nil)", paused, err)
	}
}

// TestResume_NamesTheMarkerOnAnUnrelatedFault: a Paused error that is not an
// unreadable plain marker -- here, a directory sitting where the marker
// would be -- must come back from Resume as is, naming the path, rather
// than being guessed at by removing something Resume did not cause and may
// not understand.
func TestResume_NamesTheMarkerOnAnUnrelatedFault(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, fileRecordingPaused)
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resume(root, time.Now())
	if err == nil {
		t.Fatal("Resume returned no error for a marker that is a directory")
	}
	if !strings.Contains(err.Error(), marker) {
		t.Errorf("Resume's error %q does not name the path %q", err, marker)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("Resume removed %s on a fault it neither caused nor understood: %v", marker, statErr)
	}
}
