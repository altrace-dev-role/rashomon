package consent

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// marker is the scope root's record-scope marker names, as the hook reads it.
func marker(t *testing.T, root string) string {
	t.Helper()
	s, _ := store.RecordScopeMarker(root)
	return s
}

// wantMirror checks the marker against consent.json's own answer: the same
// scope, the marker present, so the hook's one small read and RECORD's fold
// of the log agree.
func wantMirror(t *testing.T, root, when string) {
	t.Helper()
	fold, err := RecordScope(root)
	if err != nil {
		t.Fatalf("%s: RecordScope: %v", when, err)
	}
	got, err := store.RecordScopeMarker(root)
	if err != nil {
		t.Errorf("%s: the marker reads with error %v, want it written", when, err)
	}
	if got != fold {
		t.Errorf("%s: the marker says %q, consent.json says %q", when, got, fold)
	}
}

// TestRS3_LatestRecordWins: the latest record that names a scope decides it,
// both ways, and the marker follows: v5 then v4 is v4, v4 then v5 is v5.
func TestRS3_LatestRecordWins(t *testing.T) {
	for _, order := range [][2]string{{RecordScopeV5, RecordScopeV4}, {RecordScopeV4, RecordScopeV5}} {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, order[0]))
		if got := marker(t, root); got != order[0] {
			t.Errorf("after %s alone the marker says %q", order[0], got)
		}
		mustAppend(t, root, record(ViaConsentTTY, disclose.ScopeVersion, steer.DisclosureVersion, order[1]))
		if got, _ := RecordScope(root); got != order[1] {
			t.Errorf("%s then %s: RecordScope = %q, want the latest, %q", order[0], order[1], got, order[1])
		}
		if got := marker(t, root); got != order[1] {
			t.Errorf("%s then %s: the marker says %q, want the latest, %q", order[0], order[1], got, order[1])
		}
	}
}

// TestRS4_MarkerMirrorsConsentJSON: after every Append -- a record that names
// a scope, one that names none (a notice, steering turned on), and the one
// that takes the file past its cap and compacts it -- the marker equals
// RecordScope(root). It is a view of the log, rebuilt from the log, not from
// the record just written.
func TestRS4_MarkerMirrorsConsentJSON(t *testing.T) {
	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
	wantMirror(t, root, "after watch at v5")
	mustAppend(t, root, record(ViaSteerOn, disclose.ScopeVersion, steer.DisclosureVersion, ""))
	wantMirror(t, root, "after a record that names no scope")
	if got := marker(t, root); got != RecordScopeV5 {
		t.Errorf("a record naming no scope moved the marker to %q, want v5 kept", got)
	}

	for i := 0; ; i++ {
		info, err := os.Stat(Path(root))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > CapBytes-1024 {
			break
		}
		r := record(ViaFirstRunNotice, disclose.ScopeVersion, 0, "")
		r.At = int64(i)
		mustAppend(t, root, r)
	}
	wantMirror(t, root, "at the cap")
	for i := 0; i < 8; i++ {
		r := record(ViaConsentTTY, disclose.ScopeVersion, 0, RecordScopeV4)
		r.At = int64(1<<40 + i)
		mustAppend(t, root, r)
	}
	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= CapBytes/2 {
		t.Fatalf("premise: consent.json is %d bytes after passing the cap; it was not compacted", info.Size())
	}
	wantMirror(t, root, "after compaction")
	if got := marker(t, root); got != RecordScopeV4 {
		t.Errorf("after compaction the marker says %q, want the latest record's v4", got)
	}
}

// failMarkerWrites makes every marker write fail for the rest of the test:
// the process dying between the consent line's sync and the marker's rename.
func failMarkerWrites(t *testing.T) {
	t.Helper()
	was := writeMarker
	writeMarker = func(string, string) error { return errors.New("crashed before the marker") }
	t.Cleanup(func() { writeMarker = was })
}

// wantNoMarker checks that a crash left no marker, which the hook reads as v4.
func wantNoMarker(t *testing.T, root, when string) {
	t.Helper()
	got, err := store.RecordScopeMarker(root)
	if got != RecordScopeV4 || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: the marker reads %q with error %v, want it absent (v4)", when, got, err)
	}
}

// TestRS8_CrashBetweenLineAndMarkerIsV4: the marker lags the log and never
// leads it. A writer removes the marker before it appends, so a crash after
// the line is durable and before the marker is written leaves no marker: v4,
// which withholds, whichever way the consent moved. Upgrade: the v5 line is
// on file and the hook still withholds. Downgrade: the v4 line is on file
// and the v5 marker of the consent before it is gone.
func TestRS8_CrashBetweenLineAndMarkerIsV4(t *testing.T) {
	t.Run("upgrade", func(t *testing.T) {
		root := t.TempDir()
		failMarkerWrites(t)
		if err := Append(root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5)); err == nil {
			t.Error("Append reported success with its marker write failing")
		}
		if got, _ := RecordScope(root); got != RecordScopeV5 {
			t.Fatalf("premise: the v5 line is not on file (RecordScope = %q)", got)
		}
		wantNoMarker(t, root, "v5 appended, marker write failed")
	})
	t.Run("downgrade", func(t *testing.T) {
		root := t.TempDir()
		mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV5))
		if got := marker(t, root); got != RecordScopeV5 {
			t.Fatalf("premise: the marker says %q after a v5 consent", got)
		}
		failMarkerWrites(t)
		if err := Append(root, record(ViaConsentTTY, disclose.ScopeVersion, steer.DisclosureVersion, RecordScopeV4)); err == nil {
			t.Error("Append reported success with its marker write failing")
		}
		if got, _ := RecordScope(root); got != RecordScopeV4 {
			t.Fatalf("premise: the v4 line is not on file (RecordScope = %q)", got)
		}
		wantNoMarker(t, root, "v4 appended over a v5 marker, marker write failed")
	})
}

// TestRS10_CompactionKeepsTheRecordScopeRecord: the record RecordScope reads
// -- the latest that names a scope -- survives compaction even when a later
// record of the same via and scope version names none and carries the higher
// steering disclosure, which would otherwise leave it kept by neither of
// compaction's other two rules.
func TestRS10_CompactionKeepsTheRecordScopeRecord(t *testing.T) {
	root := t.TempDir()
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, 0, RecordScopeV5))
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, steer.DisclosureVersion, ""))
	if got, _ := RecordScope(root); got != RecordScopeV5 {
		t.Fatalf("premise: RecordScope = %q before compaction, want v5", got)
	}
	for i := 0; ; i++ {
		info, err := os.Stat(Path(root))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > CapBytes-1024 {
			break
		}
		r := record(ViaFirstRunNotice, disclose.ScopeVersion, 0, "")
		r.At = int64(i)
		mustAppend(t, root, r)
	}
	for i := 0; i < 8; i++ {
		mustAppend(t, root, record(ViaFirstRunNotice, disclose.ScopeVersion, 0, ""))
	}
	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= CapBytes/2 {
		t.Fatalf("premise: consent.json is %d bytes after passing the cap; it was not compacted", info.Size())
	}
	if got, _ := RecordScope(root); got != RecordScopeV5 {
		t.Errorf("RecordScope = %q after compaction, want v5: compaction dropped the record it reads", got)
	}
	if got := marker(t, root); got != RecordScopeV5 {
		t.Errorf("the marker says %q after compaction, want v5", got)
	}
}
