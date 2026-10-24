package shape

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strings"
	"testing"
)

// runnerListPins is, per RulesVersion, the sha256 of the runner lists that
// version decides verb_class and status_masked with (runnerListsDigest). A
// bump adds a pin; a pin is never edited.
var runnerListPins = map[int]string{
	1: "522cdda1bbddd626a4bb17c200854d3b31ab80bfb1c87d356bf638c9ee2d2aea",
	2: "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // rake, composer, mix, bazel test as test runners (2026-10-23)
	3: "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 3 changed the in-place tables only; the runner lists are unchanged
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

// writeTablePins is, per RulesVersion, the sha256 of the in-place tables that
// version decides may_write with (writeTablesDigest). A bump adds a pin; a
// pin is never edited. Version 1 had no in-place tables, and version 2's pin
// was computed from its tables when this test was added, with shortClusters
// empty, not read from a version-2 build.
var writeTablePins = map[int]string{
	2: "9c73ce1ce2a8d00e67481165c1adbecf0e55f0e1dfd7a1f314d62e2ba05168c5",
	3: "71c7a0e23ac2a79834dbf5d78e18ef67f4f5bc3ea663cac77aec017c38f0f7f7", // short-option clusters for sed and perl, BSD sed's -I; help and version words clear an editor (2026-10-24)
}

// writeTablesDigest digests inPlaceEditors (each editor in sorted order, then
// its words), then checkOnly, then shortClusters (each editor in sorted
// order, then its in-place letters and the letters that consume), with
// runnerListsDigest's separators.
func writeTablesDigest() string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(inPlaceEditors)) {
		b.WriteString(strings.Join(append([]string{k}, inPlaceEditors[k]...), "\x1f"))
		b.WriteByte('\x1e')
	}
	b.WriteByte('\x1d')
	b.WriteString(strings.Join(checkOnly, "\x1f"))
	b.WriteByte('\x1d')
	for _, k := range slices.Sorted(maps.Keys(shortClusters)) {
		c := shortClusters[k]
		b.WriteString(strings.Join([]string{k, c.letters, c.consumes}, "\x1f"))
		b.WriteByte('\x1e')
	}
	b.WriteByte('\x1d')
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_WriteTables: the in-place tables cannot change without a
// RulesVersion bump, or records written before and after the change carry
// the same rules_version and read as decided by the same tables. Break: add,
// drop or edit an editor, its words, a check word or a cluster and leave
// RulesVersion as it was.
func TestRulesVersion_WriteTables(t *testing.T) {
	got := writeTablesDigest()
	want, ok := writeTablePins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the in-place tables digest to %s, and RulesVersion %d pins %q: a table changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the runner-list and pass-list pins).", got, RulesVersion, want)
	}
}
