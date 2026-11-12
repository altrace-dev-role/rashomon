package hook

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// withSessionContent sets ConsentSessionContent for the rest of the test.
func withSessionContent(t *testing.T, f func(string) (bool, error)) {
	t.Helper()
	was := ConsentSessionContent
	ConsentSessionContent = f
	t.Cleanup(func() { ConsentSessionContent = was })
}

// TestSessionStartSyncsTheSessionContentMarker (W-M2.11 step 1d):
// syncSessionContent writes the marker where consent.json's answer is yes,
// removes it where the answer is no, removes it and reports false when
// consent.json cannot be read, and with no reader handed in leaves the marker
// as it was.
func TestSessionStartSyncsTheSessionContentMarker(t *testing.T) {
	yes := func(string) (bool, error) { return true, nil }
	no := func(string) (bool, error) { return false, nil }
	unreadable := func(string) (bool, error) { return false, errors.New("unreadable") }
	for _, tc := range []struct {
		name           string
		reader         func(string) (bool, error)
		before         bool
		wantOK, wantOn bool
	}{
		{"yes, no marker", yes, false, true, true},
		{"yes, a marker", yes, true, true, true},
		{"no, a marker", no, true, true, false},
		{"no, no marker", no, false, true, false},
		{"unreadable, a marker", unreadable, true, false, false},
		{"no reader, a marker", nil, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.before {
				if err := os.WriteFile(filepath.Join(root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			withSessionContent(t, tc.reader)
			if got := syncSessionContent(root); got != tc.wantOK {
				t.Errorf("sync reported %v, want %v", got, tc.wantOK)
			}
			if got := store.SessionContentMarker(root); got != tc.wantOn {
				t.Errorf("marker after the sync reads %v, want %v", got, tc.wantOn)
			}
		})
	}
}
