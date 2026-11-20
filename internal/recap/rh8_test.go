package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestRH8j_ClaimUnbackedTruthTable (item 8, card 8b): ClaimUnbacked, which
// the report's strict-claims line reads and the done flag's claim term
// (unbacked) calls, holds for a strict claim whose verdict is a gate state
// other than green, and for nothing else: a loose claim, a green one,
// no_gate_known (a claim no gate checked, which the flag reads as backed,
// D79) and a word outside the vocabulary are backed. Each want is written
// here per verdict and strictness, never read from unbacked, which calls
// ClaimUnbacked, so the test cannot pass by comparing the predicate with
// itself. Every state of verify.States has a row, so a new state needs one.
// That is one term of four: the flag also reads the row, the row's sentence
// and the unmet expected kinds, so the line is not what the Stop flagged
// (cmd/rashomon's TestRH8k). Break: flag a loose claim, a green one, a
// no_gate_known one or a word outside the states, or none.
func TestRH8j_ClaimUnbackedTruthTable(t *testing.T) {
	// strict is the want for a strict claim; a loose claim is backed on
	// every verdict.
	table := []struct {
		verdict verify.Verdict
		strict  bool
	}{
		{"red", true},
		{"masked_inconclusive", true},
		{"backgrounded", true},
		{"stale", true},
		{"absent", true},
		{"unknown", true},
		{"green", false},
		{"no_gate_known", false},
		{"not_a_state", false},
	}
	rows := map[verify.Verdict]bool{}
	for _, r := range table {
		rows[r.verdict] = true
	}
	for _, s := range verify.States {
		if !rows[verify.Verdict(s)] {
			t.Errorf("premise: state %q has no row in the table", s)
		}
	}
	for _, r := range table {
		for _, strict := range []bool{false, true} {
			c := verify.ClaimVerdict{Class: verify.ClaimTestsPass, Strict: strict, Verdict: r.verdict}
			want := strict && r.strict
			if got := ClaimUnbacked(c); got != want {
				t.Errorf("verdict %q strict %v: ClaimUnbacked %v, want %v", r.verdict, strict, got, want)
			}
			if got := unbacked([]verify.ClaimVerdict{c}); got != want {
				t.Errorf("verdict %q strict %v: the done flag's unbacked %v, want %v", r.verdict, strict, got, want)
			}
		}
	}
	red := verify.ClaimVerdict{Class: verify.ClaimTestsPass, Strict: true, Verdict: "red"}
	green := verify.ClaimVerdict{Class: verify.ClaimTestsPass, Strict: true, Verdict: "green"}
	for _, c := range []struct {
		name   string
		claims []verify.ClaimVerdict
		want   bool
	}{
		{"no claim", nil, false},
		{"a green claim alone", []verify.ClaimVerdict{green}, false},
		{"a red claim after a green one", []verify.ClaimVerdict{green, red}, true},
	} {
		if got := unbacked(c.claims); got != c.want {
			t.Errorf("%s: the done flag's unbacked %v, want %v", c.name, got, c.want)
		}
	}
}
