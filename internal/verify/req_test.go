package verify

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// reqOf is the requirement row pred:arg as the Stop's caller fills it.
func reqOf(pred, arg string, holds, known bool) ReqRow {
	return ReqRow{ID: pred + ":" + arg, Pred: pred, Arg: arg, Holds: holds, Known: known}
}

// reqSpec is a trusted spec declaring the gate spec:unit, go test ./...
func reqSpec() *spec.Compiled { return dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./...")) }

// reqRun is a run of calls, its Bash calls recorded as go test ./... in
// /repo, so the declared gate spec:unit binds them.
func reqRun(t *testing.T, calls ...call) *store.Run {
	t.Helper()
	r := runOf(t, calls...)
	for _, c := range calls {
		if c.tool == "Bash" && c.runner != "" {
			recorded(t, r, c.seq, "go test ./...")
		}
	}
	ranIn(r, "/repo", "")
	return r
}

// reqStop is the Stop at dgPoint(10) over reqRun's run of calls under sp,
// with in's Key set to dgKey.
func reqStop(t *testing.T, sp *spec.Compiled, in StopInput, calls ...call) StopView {
	t.Helper()
	in.Key = dgKey
	return EvaluateStop(reqRun(t, calls...), nil, sp, dgPoint(10), time.Time{}, in)
}

// reqRowsOf is the rows of v's ledger that name a requirement.
func reqRowsOf(v StopView) []Obligation {
	var out []Obligation
	for _, o := range v.Obligations {
		if o.Req != "" {
			out = append(out, o)
		}
	}
	return out
}

// TestReq_RowPerReqCarriesItsID: each requirement the caller checked is one
// expected_evidence row (W-M3.4): source requirement, keyed by its
// family in the point's scope, its id in Req, no evidence cited; met where
// the check holds, unmet where it was made and does not, unknown where it
// could not be made (INV-2). Nothing else moves: the Stop with its
// requirement rows taken out is the Stop with Reqs nil, every field, so no
// reader of the view (the done flag, the conflict lines' pairing over the
// ledger) can see them except as rows.
func TestReq_RowPerReqCarriesItsID(t *testing.T) {
	calls := []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)}
	reqs := []ReqRow{
		reqOf(PredFileExists, "pkg/a.go", true, true),
		reqOf(PredSymbolExists, "pkg.Gone", false, true),
		reqOf(PredTestNamed, "TestNone", true, false),
	}
	want := []ObligationState{ObligationMet, ObligationUnmet, ObligationUnknown}
	bare := reqStop(t, reqSpec(), StopInput{}, calls...)
	v := reqStop(t, reqSpec(), StopInput{Reqs: reqs}, calls...)
	scope := pointScope(v.Snap)
	if n := len(reqRowsOf(v)); n != len(reqs) {
		t.Fatalf("%d rows name a requirement, want %d: %+v", n, len(reqs), v.Obligations)
	}
	for i, q := range reqs {
		var rows []Obligation
		for _, o := range v.Obligations {
			if o.Req == q.ID {
				rows = append(rows, o)
			}
		}
		if len(rows) != 1 {
			t.Errorf("%s: %d rows, want 1", q.ID, len(rows))
			continue
		}
		o := rows[0]
		if o.ID != ObligationID(scope, ExpectedEvidence, ReqFamily(q.ID)) || o.Scope != scope || o.Kind != ExpectedEvidence ||
			o.Source != SourceRequirement || o.State != want[i] || o.Evidence == nil || len(o.Evidence) != 0 {
			t.Errorf("%s: row %+v; want id %s, scope %s, kind %s, source %s, state %s, evidence []", q.ID, o,
				ObligationID(scope, ExpectedEvidence, ReqFamily(q.ID)), scope, ExpectedEvidence, SourceRequirement, want[i])
		}
	}
	less := v
	less.Obligations = slices.DeleteFunc(slices.Clone(v.Obligations), func(o Obligation) bool { return o.Source == SourceRequirement })
	if !reflect.DeepEqual(less, bare) {
		t.Errorf("the Stop with Reqs filled, its requirement rows taken out, differs from the Stop with Reqs nil:\nfilled %+v\nnil    %+v", less, bare)
	}
}

// TestReq_OneRowPerID: two requirements of one id are one row, the first
// kept; a second row would take the same row id.
func TestReq_OneRowPerID(t *testing.T) {
	reqs := []ReqRow{reqOf(PredFileExists, "pkg/a.go", true, true), reqOf(PredFileExists, "pkg/a.go", false, true)}
	v := reqStop(t, reqSpec(), StopInput{Reqs: reqs}, edit(1, "pkg/a.go"))
	if got := reqRowsOf(v); len(got) != 1 || got[0].State != ObligationMet {
		t.Errorf("rows %+v, want one, met (the first)", got)
	}
}

// TestReq_GateGreenIgnoresCallerHolds: a gate_green requirement reads the
// Stop's own row for the gate whose id it names, never the caller's Holds or
// Known: green is met, red is unmet, and a gate edited and never run is open,
// not seen done; no row of that id is unknown, and so is every row of an
// incomplete record (INV-2, as expected's rows read) and a row of a source
// outside the four, as the ledger's own gate rows read. Each case's gate row
// or record is checked first, so the fixture reads as named. Every state of
// States is then read on reqState itself, its state written out here, not
// read from gateAfterWriteTable: stale unmet, backgrounded open, and
// masked_inconclusive, unknown and a state the table does not hold unknown.
func TestReq_GateGreenIgnoresCallerHolds(t *testing.T) {
	cases := []struct {
		name  string
		calls []call
		req   ReqRow
		gate  State // the spec:unit row's state; "" for no check
		want  ObligationState
	}{
		{"green, the caller says not known", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)},
			reqOf(PredGateGreen, "spec:unit", false, false), Green, ObligationMet},
		{"red, the caller says it holds", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed)},
			reqOf(PredGateGreen, "spec:unit", true, true), Red, ObligationUnmet},
		{"never run, the caller says it holds", []call{edit(1, "pkg/a.go")},
			reqOf(PredGateGreen, "spec:unit", true, true), Absent, ObligationOpen},
		{"no such gate, the caller says it holds", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)},
			reqOf(PredGateGreen, "spec:other", true, true), "", ObligationUnknown},
	}
	for _, c := range cases {
		v := reqStop(t, reqSpec(), StopInput{Reqs: []ReqRow{c.req}}, c.calls...)
		if c.gate != "" {
			i := slices.IndexFunc(v.Snap.Gates, func(g GateState) bool { return g.Gate == "spec:unit" })
			if i < 0 || v.Snap.Gates[i].State != c.gate {
				t.Fatalf("%s: premise: spec:unit reads %+v, want %s", c.name, v.Snap.Gates, c.gate)
			}
		}
		if got := reqRowsOf(v); len(got) != 1 || got[0].State != c.want {
			t.Errorf("%s: rows %+v, want one, %s", c.name, got, c.want)
		}
	}

	r := reqRun(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	r.Coverage = nil
	v := EvaluateStop(r, nil, reqSpec(), dgPoint(10), time.Time{}, StopInput{Key: dgKey,
		Reqs: []ReqRow{reqOf(PredGateGreen, "spec:unit", true, true)}})
	if v.Snap.Recording != Incomplete {
		t.Fatalf("incomplete record: premise: the record reads %s", v.Snap.Recording)
	}
	if got := reqRowsOf(v); len(got) != 1 || got[0].State != ObligationUnknown {
		t.Errorf("incomplete record, the caller says it holds: rows %+v, want one, unknown", got)
	}

	// EvaluateStop lists no gate row on an incomplete record (Evaluate
	// returns before the gates), so the record and source halves of the rule
	// are read here on reqState itself, over a green row of the gate: met
	// only where the ledger's own rows for it could say.
	green := GateState{Gate: "spec:unit", Family: familyDeclared, Source: sourceDeclared, State: Green}
	odd := green
	odd.Source = "elsewhere"
	q := reqOf(PredGateGreen, "spec:unit", false, false)
	for _, c := range []struct {
		name string
		snap Snapshot
		want ObligationState
	}{
		{"complete, declared", Snapshot{Recording: Complete, Gates: []GateState{green}}, ObligationMet},
		{"incomplete", Snapshot{Recording: Incomplete, Gates: []GateState{green}}, ObligationUnknown},
		{"a source outside the four", Snapshot{Recording: Complete, Gates: []GateState{odd}}, ObligationUnknown},
	} {
		if got := reqState(&c.snap, q); got != c.want {
			t.Errorf("reqState, %s green row: %s, want %s", c.name, got, c.want)
		}
	}

	// A complete record's declared row in each state, with no write cited:
	// absent reads open with or without a write.
	for _, c := range []struct {
		in   State
		want ObligationState
	}{
		{Green, ObligationMet},
		{Red, ObligationUnmet},
		{Stale, ObligationUnmet},
		{Absent, ObligationOpen},
		{Backgrounded, ObligationOpen},
		{MaskedInconclusive, ObligationUnknown},
		{Unknown, ObligationUnknown},
		{State("not_a_state"), ObligationUnknown},
	} {
		row := green
		row.State = c.in
		snap := Snapshot{Recording: Complete, Gates: []GateState{row}}
		if got := reqState(&snap, q); got != c.want {
			t.Errorf("reqState, a %s row: %s, want %s", c.in, got, c.want)
		}
	}
}

// TestReq_GateGreenIsTheTableRow: for every state of States, a gate_green
// requirement reads the state gateAfterWriteTable gives the gate's own row,
// so the requirement row and the gate's row of one ledger never disagree. A
// state added to States with no row in the table, or a second mapping in
// reqState, turns this red.
func TestReq_GateGreenIsTheTableRow(t *testing.T) {
	q := reqOf(PredGateGreen, "spec:unit", true, true)
	for _, s := range States {
		i := slices.IndexFunc(gateAfterWriteTable, func(r obRow) bool { return r.in == s })
		if i < 0 {
			t.Errorf("state %s has no row in gateAfterWriteTable", s)
			continue
		}
		snap := Snapshot{Recording: Complete, Gates: []GateState{{Gate: "spec:unit", Family: familyDeclared, Source: sourceDeclared, State: s}}}
		if got, want := reqState(&snap, q), gateAfterWriteTable[i].out; got != want {
			t.Errorf("reqState, a %s row: %s, want the table's %s", s, got, want)
		}
	}
}

// TestReq_UnmetExpectedNamesNoRequirementRow: the done flag's clause of
// unmet kinds (UnmetExpected) and the task's rows (Expected) read the same
// with requirement rows as without, while a requirement row is unmet beside
// an unmet task kind: no requirement enters the clause. The count of
// Expected rows is logged.
func TestReq_UnmetExpectedNamesNoRequirementRow(t *testing.T) {
	g := dgGate("unit", "go", "test", "./...")
	g.Gate.Supplies = []string{"real_data_run"}
	sp := dgSpec(spec.Trusted, g)
	sp.Task = &spec.Task{ID: "t1", Goal: "g", Decision: "d", ExpectedEvidence: []string{"real_data_run", "cross_check", "judgment_disclosed"}}
	calls := []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)}
	in := StopInput{Head: []byte("All 42 tests pass. The fix is complete.")}
	bare := reqStop(t, sp, in, calls...)
	in.Reqs = []ReqRow{reqOf(PredSymbolExists, "pkg.Gone", false, true), reqOf(PredGateGreen, "spec:unit", false, false)}
	v := reqStop(t, sp, in, calls...)
	if !slices.ContainsFunc(reqRowsOf(v), func(o Obligation) bool { return o.State == ObligationUnmet }) {
		t.Fatalf("premise: no requirement row is unmet: %+v", reqRowsOf(v))
	}
	u0, u1 := UnmetExpected(bare.Expected, bare.Obligations), UnmetExpected(v.Expected, v.Obligations)
	if len(u0) == 0 {
		t.Fatalf("premise: no task kind is unmet: %+v", bare.Expected)
	}
	if !slices.Equal(v.Expected, bare.Expected) {
		t.Errorf("Expected moved with requirement rows:\nnil    %+v\nfilled %+v", bare.Expected, v.Expected)
	}
	if !slices.Equal(u1, u0) {
		t.Errorf("UnmetExpected moved with requirement rows:\nnil    %+v\nfilled %+v", u0, u1)
	}
	t.Logf("%d expected rows, %d unmet, beside %d requirement rows", len(v.Expected), len(u1), len(reqRowsOf(v)))
}
