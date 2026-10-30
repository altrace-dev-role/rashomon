package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// TestSV1h_MarkFiredWritesOnceAndRefusesOtherNames: an event's first fire
// in a session creates runs/<s>/fired-<Event> (0600) holding that time, a
// later fire leaves it as it is, and a name outside FiredEvents writes
// nothing. Activity reads the markers back in FiredEvents order, beside
// whether and when the run's records file grew; a file named like a marker
// for no event is not one. Break: MarkFired rewrites the marker on every
// fire, and the time read back is the last fire's.
func TestSV1h_MarkFiredWritesOnceAndRefusesOtherNames(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, later := time.UnixMilli(1_793_000_000_000), time.UnixMilli(1_793_000_090_000)
	for _, at := range []time.Time{first, later} {
		if err := s.MarkFired("s1", "Stop", at); err != nil {
			t.Fatalf("MarkFired(Stop): %v", err)
		}
	}
	marker := filepath.Join(s.RunDir("s1"), FileFiredPrefix+"Stop")
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != strconv.FormatInt(first.UnixMilli(), 10)+"\n" {
		t.Errorf("fired-Stop = %q (%v), want the first fire's time", b, err)
	}
	if info, err := os.Stat(marker); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("fired-Stop mode %v, want 0600", info.Mode().Perm())
	}
	for _, bad := range []string{"", "stop", "../x", "Stop/..", "Notification"} {
		if err := s.MarkFired("s1", bad, first); !errors.Is(err, ErrFiredEvent) {
			t.Errorf("MarkFired(%q) = %v, want ErrFiredEvent", bad, err)
		}
	}
	entries, err := os.ReadDir(s.RunDir("s1"))
	if err != nil || len(entries) != 1 {
		t.Errorf("run directory holds %d entries (%v), want the one marker", len(entries), err)
	}

	a, err := s.Activity("s1")
	if err != nil || a.Calls || !a.LastAt.IsZero() || !slices.Equal(a.Fired, []string{"Stop"}) {
		t.Errorf("Activity before a call = %+v (%v), want no calls and [Stop]", a, err)
	}
	if err := s.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, SessionID: "s1", ToolUseID: "tu-1", ToolName: "Bash"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkFired("s1", "PreToolUse", later); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.RunDir("s1"), FileFiredPrefix+"Bogus"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.RunDir("s1"), FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.Activity("s1")
	if err != nil || !a.Calls || !a.LastAt.Equal(info.ModTime()) || !slices.Equal(a.Fired, []string{"PreToolUse", "Stop"}) {
		t.Errorf("Activity after a call = %+v (%v), want calls at %v and [PreToolUse Stop]", a, err, info.ModTime())
	}

	// A session id that is not a plain segment lands under its digest.
	if err := s.MarkFired("a/b", "SessionStart", first); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Activity(s.DirName("a/b")); err != nil || !slices.Equal(a.Fired, []string{"SessionStart"}) {
		t.Errorf("Activity(digest of a/b) = %+v (%v), want [SessionStart]", a, err)
	}
}

// TestSV1h_IsFiredFileNamesTheEightMarkersOnly: purge and the inventory
// take a run file as a marker by this, so only the eight exact names pass.
// Break: a prefix match, and a user's fired-notes.txt is purge's to delete.
func TestSV1h_IsFiredFileNamesTheEightMarkersOnly(t *testing.T) {
	for _, e := range FiredEvents() {
		if !IsFiredFile(FileFiredPrefix + e) {
			t.Errorf("IsFiredFile(%q) = false", FileFiredPrefix+e)
		}
	}
	for _, name := range []string{FileFiredPrefix, "fired-stop", "fired-Stop.tmp", "fired-notes.txt", "Stop", FileRecords} {
		if IsFiredFile(name) {
			t.Errorf("IsFiredFile(%q) = true", name)
		}
	}
	if len(FiredEvents()) != 8 {
		t.Errorf("FiredEvents has %d events, want 8", len(FiredEvents()))
	}
}

// TestSV1l_HarnessTotalsSumsEveryKey: status's harness line reads every key
// harness_counts.json holds, with its seen and stood-down sessions summed
// over the days it keeps; a file never written is none, and one that does
// not parse is an error, never none. Break: read one key only, and a
// cursor stand-down is never shown.
func TestSV1l_HarnessTotalsSumsEveryKey(t *testing.T) {
	root := t.TempDir()
	if got, err := HarnessTotals(root); len(got) != 0 || err != nil {
		t.Fatalf("no file: (%v, %v), want (none, nil)", got, err)
	}
	day1 := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for i, at := range []time.Time{day1, day1.Add(48 * time.Hour)} {
		if _, err := RecordHarnessSeen(root, "claude@unknown", "sess-"+strconv.Itoa(i), at); err != nil {
			t.Fatal(err)
		}
	}
	for i, at := range []time.Time{day1, day1, day1.Add(24 * time.Hour)} {
		if _, err := RecordHarnessMismatch(root, "claude@cursor", "sess-c"+strconv.Itoa(i), []string{"cursor_version"}, at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := HarnessTotals(root)
	want := []HarnessTotal{{Key: "claude@cursor", Seen: 3, StoodDown: 3}, {Key: "claude@unknown", Seen: 2}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("HarnessTotals = (%+v, %v), want %+v", got, err, want)
	}
	if err := os.WriteFile(filepath.Join(root, FileHarnessCounts), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := HarnessTotals(root); err == nil {
		t.Errorf("a corrupt file read as (%v, nil), want an error", got)
	}
}
