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
	2: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 2 changed runner lists only; the pass words are unchanged
	3: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 3 changed the in-place tables only; the pass words are unchanged
	4: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 4 changed how the runner lists are matched; the pass words are unchanged
	5: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 5 changed the leading-cd fold and the location rule; the pass words are unchanged
	6: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 6 changed -l in checkOnly, make's -j and the flag-table pins; the pass words are unchanged
	7: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 7 changed when a runner's line is whole and perl's -0 cluster; the pass words are unchanged
	8: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 8 added the hiding flags and folded pushd; the pass words are unchanged
	9: "59f8b278949ee9da2257c5ca139381bc0ea341c29f05278f39a7f0e1a354cab1", // shape.RulesVersion 9 ended a comment at its newline in the program search; the pass words are unchanged
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
			"Bump shape.RulesVersion and add its pin here (and the shape package's runner-list, write-table and matcher pins, and hook's location pin).", got, shape.RulesVersion, want)
	}
}
