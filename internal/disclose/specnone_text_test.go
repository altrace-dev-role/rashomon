package disclose

import (
	"strings"
	"testing"
)

// TestSR1z_StoredNamesTheSpecNoneMarker: the file the inventory lists for the
// no-spec marker (specNoneFiles) is also said in the install screen's STORED
// paragraph, and says what it holds and no more: the turn's id and a keyed
// digest of the directory. Break: drop the sentence from STORED.
func TestSR1z_StoredNamesTheSpecNoneMarker(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, StoredHeading+" for each tool call") {
			const want = "when a prompt's preflight finds no spec, one small hint that holds the turn's id and a keyed digest of the directory it looked from, nothing else"
			if !strings.Contains(p.Text, want) {
				t.Errorf("STORED does not say %q:\n%s", want, p.Text)
			}
			return
		}
	}
	t.Fatal("no STORED paragraph")
}
