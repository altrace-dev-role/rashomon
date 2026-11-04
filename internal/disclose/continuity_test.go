package disclose

import (
	"strings"
	"testing"
)

// COMPACT-SLOT1 (mgr 11-02): the briefing printed after a compaction quotes
// the person's earlier instructions, and its steer line keeps each quote's
// length and keyed digest, not the words. The two places that say a note is
// kept as shown, the STORED paragraph and steer.ndjson's row, carry the same
// exception in the same words, after the v4 preflight one. Section 3's
// example of the briefing waits for DIRECTIVE-KEEP1's body (mgr 11-02): until
// Read returns a quote no briefing prints, and the screen says only what the
// binary does.
func TestCSL1_TheScreenAndTheRowSayTheBriefingKeepsNoWords(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const (
		v4       = "except a request's preflight note at record scope v4, which keeps a count of the paths it named and keyed digests of them"
		briefing = "except the briefing after Claude Code compacts the conversation, which quotes your earlier instructions to the agent word for word; the record keeps only each quote's length and a keyed digest, never your words"
	)
	excepts := v4 + ", and " + briefing
	var stored string
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, StoredHeading+" for each tool call") {
			stored = p.Text
		}
	}
	if !strings.Contains(stored, "Each note it adds, word for word, "+excepts+".") {
		t.Errorf("the STORED paragraph's note claim lacks the briefing's exception after the v4 one: %q", stored)
	}
	for _, f := range inv.Files {
		if f.Pattern == "steer.ndjson" {
			if !strings.Contains(f.Holds, "the message text shown ("+excepts+")") {
				t.Errorf("steer.ndjson's Holds lacks the briefing's exception in the STORED paragraph's words: %q", f.Holds)
			}
			return
		}
	}
	t.Error("no steer.ndjson row in the inventory")
}
