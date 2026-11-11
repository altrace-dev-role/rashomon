package verify

import (
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestRuleKinds_UnenforcedChangeNoVerdictOrFinding pins the known limit
// spec.UnenforcedRuleKinds names: a rule of a kind Parse accepts and nothing
// enforces adds no verdict, no ledger row, no finding and no hit, with paths
// or without, at Stop and at a post. A control rule of an enforced kind
// changes both reads, so equal is not equal by construction. The card that
// enforces a kind deletes it from the list, and this test stops reading it.
func TestRuleKinds_UnenforcedChangeNoVerdictOrFinding(t *testing.T) {
	const line = "git push -f origin main"
	run := withLines(runOf(t, edit(1, "db/migrations/001.sql"), sh(2, line)), map[int64]string{2: line})
	at := RuleCall{Seq: 7, ToolName: "Edit", FilePath: "/repo/db/migrations/001.sql", Project: "/repo"}
	type read struct {
		Verdicts []RuleVerdict
		Rows     []Obligation
		Findings []steer.Finding
		Hits     []RuleHit
	}
	readWith := func(extra ...spec.Rule) read {
		sp := ruleSpec(spec.Trusted, append([]spec.Rule{noForce(), frozen(allModes...)}, extra...)...)
		snap, obs := specLedgerAt(run, sp, stopPoint(run))
		v := RulesAtCall(sp, at, atPost(7), postEvent)
		return read{snap.SpecVerdicts, obs, v.Findings, v.Hits}
	}

	base := readWith()
	if len(base.Verdicts) != 2 || len(base.Findings) != 1 {
		t.Fatalf("base: %d verdicts and %d findings, want 2 and 1: %+v", len(base.Verdicts), len(base.Findings), base)
	}
	if again := readWith(); !reflect.DeepEqual(again, base) {
		t.Fatalf("the base read twice differs:\n%+v\n%+v", base, again)
	}
	ctl := frozen(allModes...)
	ctl.ID = "ctl"
	if c := readWith(ctl); reflect.DeepEqual(c.Verdicts, base.Verdicts) || reflect.DeepEqual(c.Findings, base.Findings) {
		t.Fatalf("an enforced control rule left verdicts or findings as the base: %+v", c)
	}

	for _, k := range spec.UnenforcedRuleKinds {
		for _, paths := range [][]string{nil, {"db/migrations/**"}} {
			r := spec.Rule{ID: "k-" + string(k), Kind: string(k), Why: "w", HardControl: "CI", Paths: paths,
				From: []string{"unit:go:db"}, To: []string{"unit:go:internal"}, Predicate: spec.PredicatePathsUntouched,
				AppliesTo: []string{"file:go:db/migrations/001.sql"}, Source: "K-1"}
			if got := readWith(r); !reflect.DeepEqual(got, base) {
				t.Errorf("%s with paths %q changed a read:\n got %+v\nbase %+v", k, paths, got, base)
			}
		}
	}
}
