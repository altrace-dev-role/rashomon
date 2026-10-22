package shape

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// runnerListPins is, per RulesVersion, the sha256 of the runner lists that
// version decides verb_class and status_masked with (runnerListsDigest). A
// bump adds a pin; a pin is never edited.
var runnerListPins = map[int]string{
	1: "522cdda1bbddd626a4bb17c200854d3b31ab80bfb1c87d356bf638c9ee2d2aea",
}

// runnerListsDigest digests testCommands, then buildCommands: each row's
// words, each row, and the two lists apart.
func runnerListsDigest() string {
	var b strings.Builder
	for _, list := range [][][]string{testCommands, buildCommands} {
		for _, row := range list {
			b.WriteString(strings.Join(row, "\x1f"))
			b.WriteByte('\x1e')
		}
		b.WriteByte('\x1d')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_RunnerLists: the runner lists cannot change without a
// RulesVersion bump, or records written before and after the change carry
// the same rules_version and read as decided by the same lists. Break: add,
// drop or edit a row of testCommands or buildCommands and leave RulesVersion
// as it was.
func TestRulesVersion_RunnerLists(t *testing.T) {
	got := runnerListsDigest()
	want, ok := runnerListPins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the runner lists digest to %s, and RulesVersion %d pins %q: a list changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the report's pass-list pin).", got, RulesVersion, want)
	}
}
