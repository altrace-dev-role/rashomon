package verify

import (
	"maps"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestOL1e_TheThenChecksAreTheRulesAtTheirDeclaration (card ORDER-LIVE1): the
// Stop reads a rule's then_checks under its call's declaration seq, the seq a
// line carries; a check of another rule, of an earlier digest of the spec
// file, or landed past the point counts nothing; two checks of one call that
// disagree, or a word other than yes or no, read maybe. Break: drop any one
// of those rules.
func TestOL1e_TheThenChecksAreTheRulesAtTheirDeclaration(t *testing.T) {
	seq := func(n int64) *int64 { return &n }
	run := &store.Run{
		Declarations: []store.Declaration{{Seq: 1, ToolUseID: "a"}, {Seq: 3, ToolUseID: "b"}, {Seq: 5, ToolUseID: "c"}, {Seq: 7, ToolUseID: "d"}},
		Executions: []store.Execution{{Seq: seq(2), ToolUseID: "a"}, {Seq: seq(4), ToolUseID: "b"}, {Seq: seq(6), ToolUseID: "c"},
			{Seq: seq(8), ToolUseID: "d"}},
		ThenChecks: []store.ThenCheck{
			{RuleID: "spec:r", SourceDigest: "d1", LandedSeq: 2, Then: store.ThenNo},
			{RuleID: "spec:r", SourceDigest: "d1", LandedSeq: 4, Then: store.ThenYes},
			{RuleID: "spec:r", SourceDigest: "d1", LandedSeq: 4, Then: store.ThenNo},
			{RuleID: "spec:r", SourceDigest: "d1", LandedSeq: 6, Then: "perhaps"},
			{RuleID: "spec:r", SourceDigest: "d0", LandedSeq: 6, Then: store.ThenNo},
			{RuleID: "spec:other", SourceDigest: "d1", LandedSeq: 6, Then: store.ThenNo},
			{RuleID: "spec:r", SourceDigest: "d1", LandedSeq: 8, Then: store.ThenYes},
		},
	}
	got := thenChecks(run, "spec:r", "d1", 6)
	if want := map[int64]tri{1: triNo, 3: triMaybe, 5: triMaybe}; !maps.Equal(got, want) {
		t.Errorf("checks %v, want %v: by declaration seq, the disagreeing pair and the unknown word maybe, the rest counting nothing", got, want)
	}
	if got := thenChecks(run, "spec:r", "", 6); got != nil {
		t.Errorf("a rule with no digest: checks %v, want none", got)
	}
}
