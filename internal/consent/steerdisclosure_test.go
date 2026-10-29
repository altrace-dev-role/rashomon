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

// steerMarker is what the steer-disclosure marker reads, and whether it is
// there.
func steerMarker(t *testing.T, root string) (int, bool) {
	t.Helper()
	n, err := store.SteerDisclosureMarker(root)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false
	}
	if err != nil {
		t.Fatalf("the steer marker is unreadable: %v", err)
	}
	return n, true
}

// TestSC2_SteerDisclosureIsTheHighestOnFile: SteerDisclosure is the highest
// steer_disclosure any record on file carries, not the latest's; no file is
// 0 with no error, and a file that cannot be read is 0 with the error.
func TestSC2_SteerDisclosureIsTheHighestOnFile(t *testing.T) {
	root := t.TempDir()
	if n, err := SteerDisclosure(root); n != 0 || err != nil {
		t.Errorf("no consent.json = (%d, %v), want (0, nil)", n, err)
	}
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, 1, RecordScopeV5))
	mustAppend(t, root, record(ViaConsentTTY, disclose.ScopeVersion, 3, ""))
	mustAppend(t, root, record(ViaFirstRunNotice, disclose.ScopeVersion, 2, ""))
	if n, err := SteerDisclosure(root); n != 3 || err != nil {
		t.Errorf("records at 1, 3, 2 = (%d, %v), want (3, nil): the highest, not the latest", n, err)
	}

	negative := t.TempDir()
	mustAppend(t, negative, record(ViaConsentTTY, disclose.ScopeVersion, -3, ""))
	if n, err := SteerDisclosure(negative); n != 0 || err != nil {
		t.Errorf("a hand-edited negative = (%d, %v), want (0, nil): a value the marker can hold", n, err)
	}
	if got, ok := steerMarker(t, negative); !ok || got != 0 {
		t.Errorf("after a negative record the steer marker reads %d (present %v), want 0", got, ok)
	}

	unreadable := t.TempDir()
	if err := os.Mkdir(Path(unreadable), 0o700); err != nil {
		t.Fatal(err)
	}
	if n, err := SteerDisclosure(unreadable); n != 0 || err == nil {
		t.Errorf("an unreadable consent.json = (%d, %v), want 0 and the error", n, err)
	}
}

// TestSC2b_CurrentIsSteerDisclosureAgainstTheBuild: Current is exactly
// "SteerDisclosure reaches this build's DisclosureVersion", in every row,
// at the boundary included.
func TestSC2b_CurrentIsSteerDisclosureAgainstTheBuild(t *testing.T) {
	v := steer.DisclosureVersion
	for _, c := range []struct {
		name  string
		files []int // steer_disclosure per record; nil: no consent.json
		want  bool
	}{
		{"no consent", nil, false},
		{"below the build", []int{v - 1}, false},
		{"at the build", []int{v}, true},
		{"above the build", []int{v + 1}, true},
		{"the highest at the build, the latest below", []int{v, v - 1}, true},
		{"none consented", []int{0, 0}, false},
	} {
		root := t.TempDir()
		for _, d := range c.files {
			mustAppend(t, root, record(ViaConsentTTY, disclose.ScopeVersion, d, ""))
		}
		n, err := SteerDisclosure(root)
		if err != nil {
			t.Fatalf("%s: SteerDisclosure: %v", c.name, err)
		}
		cur, err := Current(root)
		if err != nil {
			t.Fatalf("%s: Current: %v", c.name, err)
		}
		if cur != c.want || cur != (n >= v) {
			t.Errorf("%s: Current = %v, SteerDisclosure = %d; want Current %v, equal to %d >= %d", c.name, cur, n, c.want, n, v)
		}
	}
}

// TestSC3_AppendRewritesBothMarkers: after every Append, both markers equal
// consent.json's own answers. The steer marker is written whatever the
// record says, 0 included, and it follows the highest value on file: an
// append that raises it moves it, and one that names a lower value leaves
// it.
func TestSC3_AppendRewritesBothMarkers(t *testing.T) {
	root := t.TempDir()
	check := func(when string, want int) {
		t.Helper()
		wantMirror(t, root, when)
		got, ok := steerMarker(t, root)
		if !ok || got != want {
			t.Errorf("%s: the steer marker reads %d (present %v), want %d", when, got, ok, want)
		}
		if n, err := SteerDisclosure(root); err != nil || n != got {
			t.Errorf("%s: the steer marker reads %d and consent.json %d (%v)", when, got, n, err)
		}
	}
	mustAppend(t, root, record(ViaFirstRunNotice, disclose.ScopeVersion, 0, ""))
	check("after a record that consents to no steering", 0)
	mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, 1, RecordScopeV5))
	check("after watch at 1", 1)
	mustAppend(t, root, record(ViaConsentTTY, disclose.ScopeVersion, 2, ""))
	check("after an append that raises it to 2", 2)
	mustAppend(t, root, record(ViaUpgradeNotice, disclose.ScopeVersion, 1, RecordScopeV4))
	check("after an append that names 1", 2)
}

// failSteerMarkerWrites makes every steer marker write fail for the rest of
// the test: the process dying between the consent line and the marker.
func failSteerMarkerWrites(t *testing.T) {
	t.Helper()
	was := writeSteerMarker
	writeSteerMarker = func(string, int) error { return errors.New("crashed before the steer marker") }
	t.Cleanup(func() { writeSteerMarker = was })
}

// TestSC3b_AFailedSteerMarkerWriteLeavesNone: the steer marker lags the log
// and never leads it. Append removes it before consent.json is touched, so
// a failed write after a durable line leaves no steer marker, which reads 0
// and so withholds, whether there was one before or not. The record-scope
// marker, an independent view, is written all the same, and Append's error
// names the steer marker and not the record scope's; the other way round,
// a failed record-scope write leaves the steer marker written.
func TestSC3b_AFailedSteerMarkerWriteLeavesNone(t *testing.T) {
	for _, c := range []struct {
		name   string
		before []int // records appended before the writes fail
	}{
		{"no marker before", nil},
		{"a marker at 1 before", []int{1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range c.before {
				mustAppend(t, root, record(ViaWatchTTY, disclose.ScopeVersion, d, RecordScopeV5))
			}
			if len(c.before) > 0 {
				if got, ok := steerMarker(t, root); !ok || got != 1 {
					t.Fatalf("premise: the steer marker reads %d (present %v), want 1", got, ok)
				}
			}
			failSteerMarkerWrites(t)
			err := Append(root, record(ViaConsentTTY, disclose.ScopeVersion, 2, RecordScopeV5))
			if !errors.Is(err, ErrSteerMarker) || errors.Is(err, ErrScopeMarker) {
				t.Errorf("Append = %v, want an error naming the steer marker alone", err)
			}
			if n, _ := SteerDisclosure(root); n != 2 {
				t.Fatalf("premise: the line at 2 is not on file (SteerDisclosure = %d)", n)
			}
			if got, ok := steerMarker(t, root); ok {
				t.Errorf("a failed steer marker write left the marker at %d, want none", got)
			}
			wantMirror(t, root, "the steer marker's write failed")
		})
	}
	t.Run("a failed record-scope write still writes the steer marker", func(t *testing.T) {
		root := t.TempDir()
		failMarkerWrites(t)
		err := Append(root, record(ViaConsentTTY, disclose.ScopeVersion, 2, RecordScopeV5))
		if !errors.Is(err, ErrScopeMarker) || errors.Is(err, ErrSteerMarker) {
			t.Errorf("Append = %v, want an error naming the record-scope marker alone", err)
		}
		if got, ok := steerMarker(t, root); !ok || got != 2 {
			t.Errorf("the steer marker reads %d (present %v), want 2: the two views are independent", got, ok)
		}
	})
}
