package verify

import "testing"

// TestIB1_TheStopShapesTheInferredCommandOnce (card INFER-BUDGET1): the
// Stop derives the inferred command's line once, to key its row, and names
// the row from that derivation, never a second one. Measured as allocations
// over the same run: the inferred gate adds less to EvaluateStop than two
// derivations of its line would. Control: it adds at least half of one, so
// the measure sees a derivation; and the row is still named by its runner.
// Break: name the row by deriving the line again.
func TestIB1_TheStopShapesTheInferredCommandOnce(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"))
	ranIn(r, "/repo", "")
	once := testing.AllocsPerRun(50, func() { lineShape(goTestAll.Argv, dgKey) })
	without := testing.AllocsPerRun(50, func() { giStop(r, nil, nil) })
	with := testing.AllocsPerRun(50, func() { giStop(r, nil, goTestAll) })
	added := with - without
	t.Logf("one derivation %.0f allocs; the inferred gate adds %.0f (%.0f against %.0f)", once, added, with, without)
	if added < once/2 {
		t.Fatalf("premise: the inferred gate adds %.0f allocs, under half of one derivation (%.0f): the measure does not see it", added, once)
	}
	if added >= 2*once {
		t.Errorf("the inferred gate adds %.0f allocs to the Stop, at least two derivations of its line (%.0f each): derive it once", added, once)
	}
	if p := giStop(r, nil, goTestAll).Programs[InferredGateID]; p != "go test" {
		t.Errorf("program %q, want the runner's row, go test", p)
	}
}
