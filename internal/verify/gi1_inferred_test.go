package verify

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The Stop judges the project's inferred test command as one gate (card
// GATE-INFER1, W1, W2', W3'): InferredGateID, Source "inferred", keyed by
// the runner as a declared gate is (shape.RunnerKey), required, with trust
// "inferred" until the index has seen a test run, and absent from the hook
// path.

var goTestAll = &InferredGate{Argv: []string{"go", "test", "./..."}}

// giStop is the Stop view over r, its declarations in prompt p1 so the turn
// counts them, at seq 10 under dgKey with inf inferred and idx as the index;
// the project key is "/repo", so the index observes.
func giStop(r *store.Run, idx *ProjectIndex, inf *InferredGate) StopView {
	return EvaluateStop(inPrompt(r, "p1"), idx, nil, dgPoint(10), time.Time{}, StopInput{PromptID: "p1", Key: dgKey, ProjectKey: "/repo", Inferred: inf})
}

// named is recorded, with the execution's runner_name set as call schema 8
// records it, so the run meets a gate at its runner's row.
func named(t *testing.T, r *store.Run, seq int64, line string) {
	t.Helper()
	recorded(t, r, seq, line)
	input, err := json.Marshal(map[string]string{"command": line})
	if err != nil {
		t.Fatal(err)
	}
	s := shape.Derive("Bash", input, dgKey)
	if s.RunnerName == nil {
		t.Fatalf("premise: %q names no runner", line)
	}
	for i := range r.Executions {
		if e := &r.Executions[i]; e.ToolUseID == callID(seq) {
			e.RunnerName = s.RunnerName
		}
	}
}

// TestGI1v_AnInferredGateNobodyRanIsAbsent: an edit and no run in a first
// session (no index). The inferred command is a required row, absent, under
// its closed id, named by its runner; the turn reads unverified, not no_gate.
// Control: with no inferred command the turn reads no_gate and there is no
// row. Break: drop the inferred gate from evaluate, or key its row by its
// words.
func TestGI1v_AnInferredGateNobodyRanIsAbsent(t *testing.T) {
	c := giStop(runOf(t, edit(1, "pkg/a.go")), nil, nil)
	if _, ok := gateNamed(c.Snap.Gates, InferredGateID); ok || c.Summary.Row != RowNoGate {
		t.Errorf("control, nothing inferred: rows %+v, row %s, want no row and no_gate", c.Snap.Gates, c.Summary.Row)
	}
	r := runOf(t, edit(1, "pkg/a.go"))
	ranIn(r, "/repo", "")
	v := giStop(r, nil, goTestAll)
	g, ok := gateNamed(v.Snap.Gates, InferredGateID)
	if !ok || g.State != Absent || !g.Required || g.Source != SourceInferred || g.Trust != SourceInferred || g.Kind != "test" {
		t.Fatalf("rows %+v, want %s absent, required, source and trust inferred, kind test", v.Snap.Gates, InferredGateID)
	}
	if v.Summary.Row != RowUnverified {
		t.Errorf("row %s, want unverified: the project names a test command nobody ran", v.Summary.Row)
	}
	if p := v.Programs[InferredGateID]; p != "go test" {
		t.Errorf("program %q, want the runner's row, go test", p)
	}
}

// TestGI1v_ARunOfItsRunnerBindsIt: `go test -count=1 ./pkg/...` after the
// edit binds the inferred `go test ./...` at its runner (GATE-BIND1's key):
// the row is green, its last run marked as other arguments, and the learned
// row of the same run is absorbed, while the index still learns the family,
// which is the promotion. Break: match the inferred gate by its words.
func TestGI1v_ARunOfItsRunnerBindsIt(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	named(t, r, 2, "go test -count=1 ./pkg/...")
	ranIn(r, "/repo", "")
	v := giStop(r, nil, goTestAll)
	if len(v.Snap.Gates) != 1 {
		t.Fatalf("rows %+v, want the inferred row alone: the learned row of its run is absorbed", v.Snap.Gates)
	}
	g := v.Snap.Gates[0]
	if g.Gate != InferredGateID || g.State != Green || g.LastRun == nil || !g.LastRun.OtherArgs {
		t.Fatalf("row %+v, want %s green from the run, its words other than the inferred ones", g, InferredGateID)
	}
	learnt := false
	for _, e := range v.Index.Entries {
		learnt = learnt || e.Kind == "test"
	}
	if !learnt {
		t.Errorf("index %+v holds no test family: the run would never promote", v.Index.Entries)
	}
}

// TestGI1v_RequiredInEverySessionLabelledUntilLearned: under an index that
// requires a family, so that an unseen learned family is not required, the
// inferred row still is, and the turn names it (W2'); its trust is
// "inferred" until the index has seen a test run, then builtin, the learned
// gates' word (W3'). Break: make the row required as an unseen family is,
// or keep the inferred trust after the index learned.
func TestGI1v_RequiredInEverySessionLabelledUntilLearned(t *testing.T) {
	now := time.Time{}.UnixMilli()
	idx := &ProjectIndex{ProjectKey: "/repo", Entries: []IndexEntry{
		{Family: "r:go-build@d:elsewhere", Kind: "build", Sessions: RequiredSessions, FirstSeen: now, LastSeen: now},
	}}
	if !idx.Required("r:go-build@d:elsewhere", now) {
		t.Fatal("premise: the index requires its build family")
	}
	r := runOf(t, edit(1, "pkg/a.go"))
	ranIn(r, "/repo", "")
	v := giStop(r, idx, goTestAll)
	g, ok := gateNamed(v.Snap.Gates, InferredGateID)
	if !ok || !g.Required || g.Trust != SourceInferred || v.Summary.Row != RowUnverified {
		t.Fatalf("rows %+v, row %s, want %s required, trust inferred, and the turn unverified", v.Snap.Gates, v.Summary.Row, InferredGateID)
	}
	learned := *goTestAll
	learned.Learned = true
	r = runOf(t, edit(1, "pkg/a.go"))
	ranIn(r, "/repo", "")
	v = giStop(r, idx, &learned)
	if g, ok = gateNamed(v.Snap.Gates, InferredGateID); !ok || !g.Required || g.Trust != trustBuiltin {
		t.Fatalf("learned: rows %+v, want %s required with trust %s", v.Snap.Gates, InferredGateID, trustBuiltin)
	}
}

// TestGI1o_TheLedgerOwesTheInferredGateUnderItsOwnSource: after an edit
// with no run, the ledger owes gate_after_write for the inferred gate,
// unmet, under source "inferred", never unknown. Break: leave the source
// out of obligationSource, and every row of the gate reads unknown.
func TestGI1o_TheLedgerOwesTheInferredGateUnderItsOwnSource(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"))
	ranIn(r, "/repo", "")
	v := giStop(r, nil, goTestAll)
	found := false
	for _, o := range v.Obligations {
		if o.Kind == GateAfterWrite && o.Source == SourceInferred {
			found = true
			if o.State == ObligationUnknown {
				t.Errorf("gate_after_write %+v reads unknown, want the table's state", o)
			}
		}
	}
	if !found {
		t.Errorf("obligations %+v hold no gate_after_write row under source inferred", v.Obligations)
	}
}
