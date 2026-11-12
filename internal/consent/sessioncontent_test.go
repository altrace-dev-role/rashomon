package consent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// contentMarker is the session-content marker's bytes, and whether it is
// there.
func contentMarker(t *testing.T, root string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, store.FileSessionContent))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// TestSessionContent_MarkerFollowsTheConsentOnFile (W-M2.11 step 1d): after
// each Append the session-content marker says what consent.json says: "1"
// where the highest scope_version on file is 6 or later and the latest record
// that names a record scope names v5, and no marker otherwise. A later v4
// record turns it off; a unit record, which names neither, leaves it on.
func TestSessionContent_MarkerFollowsTheConsentOnFile(t *testing.T) {
	unit := Record{V: RecordVersion, Via: ViaConsentYes, Unit: Units[0], UnitOn: true,
		TextSHA256: strings.Repeat("a", 64), BinaryVersion: "test", BinarySHA256: strings.Repeat("b", 64),
		InvocationID: strings.Repeat("c", 32), At: 1760000000000}
	for _, tc := range []struct {
		name string
		recs []Record
		want bool
	}{
		{"scope 6 at v5", []Record{record(ViaConsentYes, 6, 2, RecordScopeV5)}, true},
		{"scope 6 at v4", []Record{record(ViaConsentYes, 6, 2, RecordScopeV4)}, false},
		{"scope 5 at v5", []Record{record(ViaConsentYes, 5, 2, RecordScopeV5)}, false},
		{"scope 6 at v5, then a v4 record", []Record{record(ViaConsentYes, 6, 2, RecordScopeV5), record(ViaConsentTTY, 5, 2, RecordScopeV4)}, false},
		{"scope 6 at v5, then a unit record", []Record{record(ViaConsentYes, 6, 2, RecordScopeV5), unit}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, r := range tc.recs {
				if err := Append(root, r); err != nil {
					t.Fatalf("Append: %v", err)
				}
			}
			if got, ok := contentMarker(t, root); ok != tc.want || ok && got != "1\n" {
				t.Errorf("marker = %q (present %v), want present %v holding \"1\\n\"", got, ok, tc.want)
			}
			if on, err := SessionContent(root); err != nil || on != tc.want {
				t.Errorf("SessionContent = %v (%v), want %v", on, err, tc.want)
			}
		})
	}
}

// TestSessionContent_MarkerFailureKeepsTheLine: Append stopped between its
// line and the marker after it -- the write that follows the durable line
// fails -- leaves the line on file and no marker, even where one was there
// before, so the hook reads no consent to keep the session's content; the
// error is ErrSessionContentMarker alone, and the other two markers are
// written.
func TestSessionContent_MarkerFailureKeepsTheLine(t *testing.T) {
	root := t.TempDir()
	if err := Append(root, record(ViaConsentYes, 6, 2, RecordScopeV5)); err != nil {
		t.Fatalf("premise: Append: %v", err)
	}
	if !store.SessionContentMarker(root) {
		t.Fatal("premise: no marker after a consent at scope 6 at v5")
	}
	was := writeSessionContentMarker
	writeSessionContentMarker = func(string) error { return errors.New("disk full") }
	t.Cleanup(func() { writeSessionContentMarker = was })

	err := Append(root, record(ViaConsentTTY, 6, 2, RecordScopeV5))
	if !errors.Is(err, ErrSessionContentMarker) || errors.Is(err, ErrScopeMarker) || errors.Is(err, ErrSteerMarker) {
		t.Fatalf("Append = %v, want ErrSessionContentMarker alone", err)
	}
	if recs, err := Read(root); err != nil || len(recs) != 2 || recs[1].Via != ViaConsentTTY {
		t.Errorf("consent.json holds %v (%v), want the second record durable", recs, err)
	}
	if store.SessionContentMarker(root) {
		t.Error("the hook reads consent to keep the session's content after a marker write that failed")
	}
	if got, ok := contentMarker(t, root); ok {
		t.Errorf("marker %q after a failed write, want none", got)
	}
	if scope, err := store.RecordScopeMarker(root); err != nil || scope != store.RecordScopeV5 {
		t.Errorf("record_scope marker = %q (%v), want v5", scope, err)
	}
}
