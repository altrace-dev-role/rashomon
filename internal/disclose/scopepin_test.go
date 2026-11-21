package disclose

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// scopeTextPins is the digest of the screen's words at each ScopeVersion,
// scopeTextDigest's hex. A change to the words moves ScopeVersion and adds
// that version's row here in the same commit; a row is never edited.
var scopeTextPins = map[int]string{
	// 6 is the words release v1.2.0-alpha.2 ships. Main changed the words
	// under 6 more than once before that release, so this row does not show
	// that the words never changed under 6: the pin starts here.
	6: "a6d718c1610ab656f4cf78fc5273017ab950ea312ecc2167371dd82ba5c861e5",
	// 7 adds the Stop's reads of a repository graph's configuration and
	// origin url, at the end of a reply that wrote a file and when a
	// session's ledger is printed.
	7: "2e78aba87f99fd4efe291f31aef30449a0a68860980a6536327465d3e4e83d9b",
	// 8 corrects the repository id's words (an empty or default port, a
	// relative origin), says when graph index writes .rashomon/graph.json
	// and what a /clear keeps, and adds the consent-file limit
	// (CONSENT-GATE1).
	8: "de69ee193c572b2b5db9bb9a63af89a7182c0ebc9b79524dc2ba37c164a29096",
}

// scopeTextDigest is the sha256 of every screen paragraph and unit
// paragraph, in order, each field kept apart by a separator no text holds.
func scopeTextDigest(t *testing.T) string {
	t.Helper()
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, p := range inv.Paragraphs {
		b.WriteString(p.Owner + "\x1f" + p.Cite + "\x1f" + p.Text + "\x1e")
	}
	b.WriteString("\x1d")
	for _, u := range inv.Units {
		b.WriteString(u.Owner + "\x1f" + u.Unit + "\x1f" + u.Cite + "\x1f" + u.Text + "\x1e")
	}
	s := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(s[:])
}

// TestScopeText_PinnedToItsVersion: the screen's words digest to their
// ScopeVersion's pin, so a sentence added or changed under an unchanged
// version fails here by name.
func TestScopeText_PinnedToItsVersion(t *testing.T) {
	got := scopeTextDigest(t)
	pin, ok := scopeTextPins[ScopeVersion]
	if !ok || pin == "" {
		t.Fatalf("no pin for version %d; the screen's words digest to %s", ScopeVersion, got)
	}
	if got != pin {
		t.Errorf("text changed under version %d: digest %s, pin %s; move ScopeVersion and add its pin", ScopeVersion, got, pin)
	}
}
