package engine

import "testing"

// TestIsKnowledge_EveryJudgmentKind (W-M1.6): the seven row kinds of the
// knowledge schema are knowledge to Scope and Rank, so a flow or an
// objective never seeds a traversal; a structure kind is not.
func TestIsKnowledge_EveryJudgmentKind(t *testing.T) {
	for _, k := range []string{"lesson", "decision", "incident", "convention", "exception", "flow", "objective"} {
		if !IsKnowledge(k) {
			t.Errorf("IsKnowledge(%q) = false", k)
		}
	}
	for _, k := range []string{"sym", "file", "unit", "repo", "tbl", "contract", ""} {
		if IsKnowledge(k) {
			t.Errorf("IsKnowledge(%q) = true", k)
		}
	}
}
