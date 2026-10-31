package purge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestCE1d_PurgeRemovesTheChannelEvidenceMarker: the marker under the store
// root, one per event, is rashomon's own file, so a purge plans it and a
// file of another name beside it stays foreign.
func TestCE1d_PurgeRemovesTheChannelEvidenceMarker(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if !s.TakeChannelEvidence("PostToolUse", "2.1.304") || !s.TakeChannelEvidence("PostToolUseFailure", "2.1.304") {
		t.Fatal("markers not taken")
	}
	if err := os.WriteFile(filepath.Join(root, "channel-evidence-not.a.marker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"PostToolUse", "PostToolUseFailure"} {
		want := filepath.Join(root, store.ChannelEvidencePrefix+ev)
		found := false
		for _, f := range p.Files {
			found = found || f == want
		}
		if !found {
			t.Errorf("purge does not plan %s", want)
		}
	}
	if len(p.Foreign) != 1 || filepath.Base(p.Foreign[0]) != "channel-evidence-not.a.marker" {
		t.Errorf("foreign = %v, want only the file that is no marker", p.Foreign)
	}
}
