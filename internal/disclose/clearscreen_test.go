package disclose

import (
	"strings"
	"testing"
)

// TestClearRendezvous_TheScreenSaysWhatAClearKeeps (M4-PR1, rulings x35.5
// and x40): one paragraph, the one for a session that starts or is cleared,
// says what the /clear rendezvous keeps, as states: the folder's name is the
// SHA-256 digest of the working directory's path, a name derived from the
// path; an empty file named by each session's id and, when the two pair, a
// file named by the new session's id that holds the old one's; when each is
// removed; and that `rashomon purge` removes them. Break: drop any clause,
// or say them in a second paragraph or another one.
func TestClearRendezvous_TheScreenSaysWhatAClearKeeps(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "Across a /clear, the store's clear directory holds"
	every := []string{
		"a folder whose name is the SHA-256 digest of the working directory's path, which anyone who can guess the path can compute",
		"an empty file named by each of the two sessions' ids",
		"when they pair, a file named by the new session's id that holds the old session's id",
		"The empty files are removed once that file is written",
		"removed by the next /clear in that folder that finds it more than an hour old",
		"`rashomon purge` removes them all",
	}
	said := 0
	for _, p := range inv.Paragraphs {
		if !strings.Contains(p.Text, anchor) {
			continue
		}
		said++
		if p.Owner != OwnerRecord || p.Cite != CiteSection5 || !strings.HasPrefix(p.Text, "When a session starts or is cleared") {
			t.Errorf("the /clear clause is in %s/%s, not the paragraph for a session that starts or is cleared", p.Owner, p.Cite)
		}
		for _, w := range every {
			if !strings.Contains(p.Text, w) {
				t.Errorf("the /clear clause does not say %q", w)
			}
		}
	}
	if said != 1 {
		t.Errorf("%d paragraphs say what a /clear keeps, want 1", said)
	}
}
