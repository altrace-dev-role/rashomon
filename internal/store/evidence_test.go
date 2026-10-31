package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestCE1d_MarkerIsCreatedOnceAndKeepsItsVersion: the marker is an O_EXCL
// create, so a second take is refused and the version that shadowed first
// stays; it is read back as the version and cleared by name.
func TestCE1d_MarkerIsCreatedOnceAndKeepsItsVersion(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ChannelEvidence("PostToolUse"); ok {
		t.Fatal("a marker exists in a fresh store")
	}
	if !s.TakeChannelEvidence("PostToolUse", "2.1.304") {
		t.Fatal("the first take was refused")
	}
	if s.TakeChannelEvidence("PostToolUse", "2.1.400") {
		t.Error("a second take created the marker again")
	}
	v, ok := s.ChannelEvidence("PostToolUse")
	if !ok || v != "2.1.304" {
		t.Errorf("marker reads (%q, %v), want (2.1.304, true)", v, ok)
	}
	if _, ok := s.ChannelEvidence("PostToolUseFailure"); ok {
		t.Error("one event's marker shadows another's")
	}
	fi, err := os.Stat(filepath.Join(root, store.ChannelEvidencePrefix+"PostToolUse"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("marker file: %v, %v; want mode 0600", fi, err)
	}
	s.ClearChannelEvidence("PostToolUse")
	if _, ok := s.ChannelEvidence("PostToolUse"); ok {
		t.Error("the marker survived ClearChannelEvidence")
	}
}

// TestCE1d_RacingTakesCreateOneMarker: of many Stops that find the same event
// absent, one creates the marker.
func TestCE1d_RacingTakesCreateOneMarker(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	won := 0
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.TakeChannelEvidence("PostToolUse", "2.1.304") {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Errorf("%d takes created the marker, want 1", won)
	}
}

// TestCE1d_OnlyAnEventNameReachesAPath: a name that is not letters never
// becomes a path, in either direction.
func TestCE1d_OnlyAnEventNameReachesAPath(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"", "../x", "a/b", "Post.Tool", "P1", strings.Repeat("a", 33)} {
		if s.TakeChannelEvidence(ev, "2.1.304") {
			t.Errorf("event %q took a marker", ev)
		}
		if _, ok := s.ChannelEvidence(ev); ok {
			t.Errorf("event %q read a marker", ev)
		}
	}
	for name, want := range map[string]bool{
		"channel-evidence-PostToolUse": true, "channel-evidence-": false, "channel-evidence-a.b": false,
		"channel-evidence": false, "notice-channel": false,
	} {
		if got := store.IsChannelEvidenceMarker(name); got != want {
			t.Errorf("IsChannelEvidenceMarker(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestCE1d_AnOverlongMarkerStillShadows: a marker that is not a short version
// is read as shadowing with no version, so only a probe pass clears it.
func TestCE1d_AnOverlongMarkerStillShadows(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, store.ChannelEvidencePrefix+"PostToolUse")
	if err := os.WriteFile(p, []byte(strings.Repeat("9", 500)), 0o600); err != nil {
		t.Fatal(err)
	}
	v, ok := s.ChannelEvidence("PostToolUse")
	if !ok || v != "" {
		t.Errorf("overlong marker reads (%q, %v), want (\"\", true)", v, ok)
	}
}
