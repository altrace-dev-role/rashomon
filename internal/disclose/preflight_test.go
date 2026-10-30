package disclose

import (
	"strings"
	"testing"
)

// TestPF1n_ScreenSaysPromptsAreReadInMemory: the screen's sentence on
// prompts says what the hook now does with one: it reads the text in memory
// to pick the project facts a note gives, never stores it, and records only
// its length. The sentence changes under the same steer disclosure version,
// so nothing is held for a new consent. Break: keep "does not examine its
// text", and the screen contradicts the preflight it ships with.
func TestPF1n_ScreenSaysPromptsAreReadInMemory(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	said := 0
	for _, p := range inv.Paragraphs {
		if strings.Contains(p.Text, "does not examine its text") {
			t.Errorf("%s/%s still says a prompt's text is not examined: %q", p.Owner, p.Cite, p.Text)
		}
		if !strings.Contains(p.Text, "reads each in memory") {
			continue
		}
		said++
		if p.Owner != OwnerRecord || p.Cite != CiteSection2 {
			t.Errorf("the prompt sentence is in %s/%s, want %s/%s", p.Owner, p.Cite, OwnerRecord, CiteSection2)
		}
		for _, want := range []string{"Your prompts are never stored", "to pick project facts for a note", "keeps only the project paths it names"} {
			if !strings.Contains(p.Text, want) {
				t.Errorf("the prompt sentence does not say %q: %q", want, p.Text)
			}
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs say a prompt is read in memory, want 1", said)
	}
}
