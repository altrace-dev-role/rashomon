package disclose

import (
	"strings"
	"testing"
)

// TestConsentGate1_FullTextSaysTheAnswersCanBeChanged (x43.4, CONSENT-GATE1):
// the full text says, in a paragraph of its own right after the one that
// begins "rashomon itself makes no network connection", that the answers on
// this screen are kept unsigned in the store and that a program running as
// the user can change them in a way this build cannot tell from the user's.
// The short screen's only paragraph text is its CiteShort paragraphs
// (consent.Short), so one paragraph saying it, a CiteSection2 one, keeps it
// off the short screen. The gate's code is CONSENT-GATE1's; the sentence
// states a limit true on main today. Break: drop the sentence, move it,
// merge it into another paragraph, or put it on the short screen.
func TestConsentGate1_FullTextSaysTheAnswersCanBeChanged(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const limit = "Your answers on this screen are kept, unsigned, in files in the store. A program that runs as you, the agent included, can change them and turn on reads you did not agree to, and this build cannot tell such a change from yours; `rashomon status` shows the screen version on file and when it was agreed."
	said, net := 0, -1
	for i, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, "rashomon itself makes no network connection") {
			net = i
		}
		if !strings.Contains(p.Text, limit) {
			continue
		}
		said++
		if p.Text != limit || p.Owner != OwnerScreen || p.Cite != CiteSection2 || net < 0 || i != net+1 {
			t.Errorf("the consent-file limit is in %s/%s as paragraph %d; want its own %s/%s paragraph right after the network paragraph (%d)", p.Owner, p.Cite, i, OwnerScreen, CiteSection2, net)
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs say the answers on file can be changed, want 1", said)
	}
}
