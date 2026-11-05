package steer

import (
	"errors"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestFinding_Schema (W-M1.7, AD19, AC-X.5): the graph line kinds are a
// closed list of ten, each with its compliance predicates named; the
// source kinds are the five store classes; and Check refuses a Source.Kind
// outside them, a Unit that is not a global id, and a Source.ID that is
// not one, while a finding carrying a graph source and a unit passes.
func TestFinding_Schema(t *testing.T) {
	want := map[Kind][]string{
		KindUnit: {PredOpenedUnit}, KindReuse: {PredCalledSymbol}, KindConsumer: {PredEditedOrVerified},
		KindDecision: {PredPathsUntouched, PredSymbolPresent}, KindLesson: {PredLessonOwn}, KindConflict: {PredResolvedByStop},
		KindObligation: {PredMet}, KindBoundary: {PredNone}, KindDrift: {PredReturnedToScope}, KindFlow: {PredConsumersVerified},
	}
	if len(GraphKinds()) != len(want) {
		t.Fatalf("%d graph kinds; want %d", len(GraphKinds()), len(want))
	}
	for _, k := range GraphKinds() {
		got, ok := Compliance(k)
		if !ok || len(got) != len(want[k]) {
			t.Errorf("Compliance(%s) = %v, %v; want %v", k, got, ok, want[k])
			continue
		}
		for i := range got {
			if got[i] != want[k][i] {
				t.Errorf("Compliance(%s)[%d] = %q; want %q", k, i, got[i], want[k][i])
			}
		}
	}
	if _, ok := Compliance("other"); ok {
		t.Errorf("Compliance(other) answered")
	}
	if len(SourceKinds()) != 5 || SourceKinds()[0] != SourceGraph || SourceKinds()[4] != SourceProposer {
		t.Errorf("source kinds = %v", SourceKinds())
	}
	prio, _ := pattern.Priority(pattern.TaskPreflight)
	base := func() Finding {
		return Finding{Pattern: pattern.TaskPreflight, Key: "graph", Priority: prio, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 3}
	}
	good := base()
	good.Source = Source{Kind: SourceGraph, ID: "g:github.com/x/y#file:jobs/purge.py", Tier: "algorithmic"}
	good.Unit = "g:github.com/x/y#unit:py:jobs"
	if err := good.Check(); err != nil {
		t.Errorf("a finding with a graph source and a unit: %v", err)
	}
	for name, edit := range map[string]func(*Finding){
		"source kind outside the five": func(f *Finding) { f.Source = Source{Kind: "model", ID: "g:r#x"} },
		"source id not global":         func(f *Finding) { f.Source = Source{Kind: SourceGraph, ID: "file:jobs/purge.py"} },
		"source kind with no id":       func(f *Finding) { f.Source = Source{Kind: SourceKnowledge} },
		"unit not global":              func(f *Finding) { f.Unit = "unit:py:jobs" },
		"unit of another node kind":    func(f *Finding) { f.Unit = "g:r#file:jobs/purge.py" },
	} {
		f := base()
		edit(&f)
		if err := f.Check(); !errors.Is(err, ErrFinding) {
			t.Errorf("%s: Check = %v; want ErrFinding", name, err)
		}
	}
	if err := base().Check(); err != nil {
		t.Errorf("a finding with no source and no unit: %v", err)
	}
}
