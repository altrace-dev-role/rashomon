package verify

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestRC15f_TheStopNamesOnlyThisTurnsBindingViolations (card RECALL-15): of
// the session's rule verdicts (the window is the run, D40), the Stop names a
// trusted or managed rule's violation with a seq in this turn, its seqs cut
// to the turn's; an untrusted rule's, a rule kept, and a violation of an
// earlier turn only are not named, and a turn with no declaration names
// none. Break: name every turn's seqs, an untrusted rule, or a turn with no
// window, and this fails.
func TestRC15f_TheStopNamesOnlyThisTurnsBindingViolations(t *testing.T) {
	v := func(id string, verdict RuleVerdictValue, trust spec.Trust, seqs ...int64) RuleVerdict {
		return RuleVerdict{Rule: SlotID(id), Kind: spec.ForbidCommand, Verdict: verdict, Seqs: seqs, trust: trust}
	}
	vs := []RuleVerdict{
		v("both-turns", Violation, spec.Trusted, 2, 9, 14),
		v("earlier-turn", Violation, spec.Trusted, 3, 8),
		v("untrusted", Violation, spec.Untrusted, 10),
		v("managed", Violation, spec.Managed, 9),
		v("kept", NoRecordedViolation, spec.Trusted),
		v("at-the-first", Violation, spec.Trusted, 4, 9),
	}
	got := turnViolations(vs, 9)
	var names []string
	for _, r := range got {
		names = append(names, string(r.Rule))
	}
	if want := []string{"both-turns", "managed", "at-the-first"}; !slices.Equal(names, want) {
		t.Fatalf("named %v, want %v", names, want)
	}
	if !slices.Equal(got[0].Seqs, []int64{9, 14}) || !slices.Equal(got[2].Seqs, []int64{9}) {
		t.Errorf("seqs %v and %v, want [9 14] and [9]: the turn's alone", got[0].Seqs, got[2].Seqs)
	}
	if !slices.Equal(vs[0].Seqs, []int64{2, 9, 14}) {
		t.Errorf("the snapshot's verdict became %v, want it as it was", vs[0].Seqs)
	}
	if got := turnViolations(vs, 0); got != nil {
		t.Errorf("a turn with no declaration named %v, want none", got)
	}
}
