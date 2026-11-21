package consent

import (
	"os"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestScopeOnFile_TheHighestScopeOnFile: ScopeOnFile is the highest
// scope_version of any record on file, whichever came last: 0 with no file,
// 6 with a lone 6, 9 with a 9 and a later 8. A later, lower record never
// takes back a scope a yes once reached, as SessionContent reads it; the
// overlay writer compares this number with a reader's own scope
// (lang.KeptFrom). An unreadable file is 0 with the error. Breaks: read the
// last record's scope; answer true or false in place of the number; answer
// a scope past the error.
func TestScopeOnFile_TheHighestScopeOnFile(t *testing.T) {
	root := t.TempDir()
	if s, err := ScopeOnFile(root); s != 0 || err != nil {
		t.Errorf("ScopeOnFile with no file = %d, %v; want 0 and no error", s, err)
	}
	mustAppend(t, root, record(ViaWatchYes, 6, steer.DisclosureVersion, RecordScopeV5))
	if s, err := ScopeOnFile(root); s != 6 || err != nil {
		t.Errorf("ScopeOnFile over a lone 6 = %d, %v; want 6", s, err)
	}
	mustAppend(t, root, record(ViaConsentYes, 9, steer.DisclosureVersion, RecordScopeV5))
	mustAppend(t, root, record(ViaConsentYes, 8, steer.DisclosureVersion, RecordScopeV5))
	if s, err := ScopeOnFile(root); s != 9 || err != nil {
		t.Errorf("ScopeOnFile over a 9 and a later 8 = %d, %v; want 9", s, err)
	}

	bad := t.TempDir()
	if err := os.Mkdir(Path(bad), 0o700); err != nil {
		t.Fatal(err)
	}
	if s, err := ScopeOnFile(bad); s != 0 || err == nil {
		t.Errorf("ScopeOnFile on a directory = %d, %v; want 0 and an error", s, err)
	}
}
