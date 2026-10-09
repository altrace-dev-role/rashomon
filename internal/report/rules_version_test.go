package report

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// passListPins is, per shape.RulesVersion, the sha256 of the pass words that
// version decides a pass claim with (passVocabulary). A bump adds a pin; a
// pin is never edited.
var passListPins = map[int]string{
	1: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1",
}

// TestRulesVersion_PassList: the pass words cannot change without a
// shape.RulesVersion bump. Break: add or drop a pass word and leave the
// version as it was.
func TestRulesVersion_PassList(t *testing.T) {
	sum := sha256.Sum256([]byte(strings.Join(passVocabulary, "\x1e")))
	got := hex.EncodeToString(sum[:])
	want, ok := passListPins[shape.RulesVersion]
	if !ok || want != got {
		t.Fatalf("the pass words digest to %s, and RulesVersion %d pins %q: the list changed without a bump. "+
			"Bump shape.RulesVersion and add its pin here (and the shape package's runner-list pin).", got, shape.RulesVersion, want)
	}
}
