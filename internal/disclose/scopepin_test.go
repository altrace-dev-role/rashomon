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
	// 6 is the words after the correction to the list of rule kinds a spec
	// cannot make change a finding. The words main carried at 6 before that
	// correction differ in that one sentence, so this row does not show that
	// the words never changed under 6: the pin starts here.
	6: "747a005dd9d4d2bfba2304e1b54617436c683f15797d4e0fb320719b367239f9",
	// 7 adds the Stop's reads of a repository graph's configuration and
	// origin url, at the end of a reply that wrote a file and when a
	// session's ledger is printed.
	7: "53deb8bcb93f4836a4f1850ecb6149c9c7af0b340ec206d95bf242958de5b56b",
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
