package intent_test

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestRuleKinds_UnenforcedIntentNamesOnlyWithPaths pins what the prompt
// reader does today with a rule of a kind in spec.UnenforcedRuleKinds. With
// no paths it names nothing new. With paths that cover the prompt's file it
// names the rule as it names any path rule, because Resolve's rule loop
// reads Covers and not the kind. The card that gives a kind its meaning
// decides this line and deletes the kind from the list in the same commit.
// A require_evidence control is named with no paths, so equal is not equal
// by construction.
func TestRuleKinds_UnenforcedIntentNamesOnlyWithPaths(t *testing.T) {
	root := projectTree(t)
	const prompt = "fix the bug in internal/verify/stop.go"
	rules := func(extra ...spec.Rule) []intent.Rule {
		sp := specOf(root, spec.Trusted)
		for _, r := range extra {
			sp.Rules = append(sp.Rules, spec.DeclaredRule{ID: "spec:" + r.ID, Rule: r})
		}
		return intent.Resolve([]byte(prompt), false, projectOf(root, sp, indexOf())).Rules
	}

	base := rules()
	if !slices.ContainsFunc(base, func(r intent.Rule) bool { return r.ID == "conv-errors" }) {
		t.Fatalf("the base names %+v, want conv-errors among them", base)
	}
	if got := rules(spec.Rule{ID: "ctl", Kind: string(spec.RequireEvidence), Why: "c"}); slices.Equal(got, base) {
		t.Fatalf("a require_evidence control left the names as the base: %+v", got)
	}

	for _, k := range spec.UnenforcedRuleKinds {
		r := spec.Rule{ID: "k-" + string(k), Kind: string(k), Why: "w", Predicate: spec.PredicatePathsUntouched}
		if got := rules(r); !slices.Equal(got, base) {
			t.Errorf("%s without paths: names %+v, want the base %+v", k, got, base)
		}
		r.Paths = []string{"internal/**"}
		want := append(slices.Clone(base), intent.Rule{ID: r.ID, Trust: spec.Trusted, Why: r.Why})
		if got := rules(r); !slices.Equal(got, want) {
			t.Errorf("%s with paths: names %+v, want %+v", k, got, want)
		}
	}
}
