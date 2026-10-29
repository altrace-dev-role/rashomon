package verify

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// obAt is the ledger at a stop point at seq, with no index and no spec.
func obAt(r *store.Run, seq int64) []Obligation {
	_, obs := EvaluateWithObligations(r, nil, nil, Point{Kind: PointStop, Seq: seq, SessionID: "s1"}, time.Time{})
	return obs
}

// obGate is a required learned gate of testFamily in state s, citing what a
// gateStates gate in s cites: the run at 10, as LastGreen too where s is
// Green or Stale, and the write at 12 where s is Stale or Absent.
func obGate(s State) GateState {
	g := GateState{Gate: "learned:test:r:go-test@d:repo", Kind: "test", Family: testFamily, Source: sourceLearned,
		Required: true, State: s, WritersSinceGreen: []AgentCount{}}
	run := &RunRef{Seq: 10}
	switch s {
	case Green:
		g.LastRun, g.LastGreen = run, run
	case Stale:
		g.LastRun, g.LastGreen, g.LastWrite = run, run, &WriteRef{Seq: 12}
	case Absent:
		g.LastWrite = &WriteRef{Seq: 12}
	default:
		g.LastRun = run
	}
	return g
}

// obSnap is a complete snapshot at a stop point at 100 over gates.
func obSnap(gates ...GateState) Snapshot {
	return Snapshot{Point: Point{Kind: PointStop, Seq: 100}, Recording: Complete, Gates: gates}
}

// rowOf is the one row of kind in obs; it fails the test unless there is
// exactly one.
func rowOf(t *testing.T, obs []Obligation, kind ObligationKind) Obligation {
	t.Helper()
	var got []Obligation
	for _, o := range obs {
		if o.Kind == kind {
			got = append(got, o)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%d %s rows, want one: %+v", len(got), kind, obs)
	}
	return got[0]
}

func noRow(t *testing.T, obs []Obligation, kind ObligationKind) {
	t.Helper()
	if i := slices.IndexFunc(obs, func(o Obligation) bool { return o.Kind == kind }); i >= 0 {
		t.Fatalf("a %s row %+v, want none", kind, obs[i])
	}
}

func wantRow(t *testing.T, o Obligation, s ObligationState, evidence ...int64) {
	t.Helper()
	if evidence == nil {
		evidence = []int64{}
	}
	if o.State != s || o.Evidence == nil || !slices.Equal(o.Evidence, evidence) {
		t.Fatalf("%s: %s %v, want %s %v", o.Kind, o.State, o.Evidence, s, evidence)
	}
}

func kindsOf(obs []Obligation) []ObligationKind {
	ks := make([]ObligationKind, len(obs))
	for i, o := range obs {
		ks[i] = o.Kind
	}
	return ks
}

func encodeObs(t *testing.T, obs []Obligation) []byte {
	t.Helper()
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestPG5a_GreenGateIsMet: a required gate green after the last relevant write
// owes three rows, in kind order: gate_after_write met on its last green run,
// exit_unmasked met on its last run, evidence_bound unknown with no evidence
// (no tree at the point; card P5A); no watched row, since no run was red. An advisory gate owes
// nothing (D36, D38), and a Green with no LastGreen is unknown (INV-2).
func TestPG5a_GreenGateIsMet(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	run := declSeq(r, 1)
	obs := obAt(r, 4)
	if want := []ObligationKind{GateAfterWrite, ExitUnmasked, EvidenceBound}; !slices.Equal(kindsOf(obs), want) {
		t.Fatalf("kinds %v, want %v", kindsOf(obs), want)
	}
	wantRow(t, rowOf(t, obs, GateAfterWrite), ObligationMet, run)
	wantRow(t, rowOf(t, obs, ExitUnmasked), ObligationMet, run)
	wantRow(t, rowOf(t, obs, EvidenceBound), ObligationUnknown)
	for _, o := range obs {
		if o.Scope != ScopeSession || o.Source != sourceLearned {
			t.Errorf("%s: scope %q source %q, want session and learned", o.Kind, o.Scope, o.Source)
		}
	}

	advisory := obGate(Green)
	advisory.Required = false
	if obs := obligations(obSnap(advisory), nil, nil); obs == nil || len(obs) != 0 {
		t.Fatalf("an advisory gate owes %+v, want an empty ledger", obs)
	}
	advisory.Gate, advisory.Family = "learned:test:r:go-vet@d:repo", vetFamily
	for _, o := range obligations(obSnap(obGate(Green), advisory), nil, nil) {
		if o.ID == ObligationID(ScopeSession, o.Kind, vetFamily) {
			t.Errorf("an advisory gate beside a required one owes %+v", o)
		}
	}

	noGreen := obGate(Green)
	noGreen.LastGreen = nil
	wantRow(t, rowOf(t, obligations(obSnap(noGreen), nil, nil), GateAfterWrite), ObligationUnknown)
}

// TestPG5b_StaleGateIsUnmet: gate_after_write by the gate's state. Stale and
// Red are unmet, Absent with a relevant write is open, Backgrounded is open,
// Masked and Unknown are unknown. An Absent gate with no write owes nothing:
// no row of any kind. Absent is unreachable from gateStates until declared
// gates land (3.1), so it is driven here through obligations() (Gate B F1). A
// cited pointer that is nil, or a state outside States, is unknown (Q2).
func TestPG5b_StaleGateIsUnmet(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go"))
	wantRow(t, rowOf(t, obAt(r, 4), GateAfterWrite), ObligationUnmet, declSeq(r, 0), declSeq(r, 1))

	for _, c := range []struct {
		state    State
		want     ObligationState
		evidence []int64
	}{
		{Red, ObligationUnmet, []int64{10}},
		{Stale, ObligationUnmet, []int64{10, 12}},
		{Absent, ObligationOpen, []int64{12}},
		{Backgrounded, ObligationOpen, []int64{10}},
		{MaskedInconclusive, ObligationUnknown, []int64{10}},
		{Unknown, ObligationUnknown, []int64{10}},
	} {
		wantRow(t, rowOf(t, obligations(obSnap(obGate(c.state)), nil, nil), GateAfterWrite), c.want, c.evidence...)
	}

	nothing := obGate(Absent)
	nothing.LastWrite = nil
	if obs := obligations(obSnap(nothing), nil, nil); len(obs) != 0 {
		t.Fatalf("an Absent gate with no write owes %+v, want nothing", obs)
	}

	noRun := obGate(Red)
	noRun.LastRun = nil
	wantRow(t, rowOf(t, obligations(obSnap(noRun), nil, nil), GateAfterWrite), ObligationUnknown)
	noWrite := obGate(Stale)
	noWrite.LastWrite = nil
	wantRow(t, rowOf(t, obligations(obSnap(noWrite), nil, nil), GateAfterWrite), ObligationUnknown)

	defect := obGate(Green)
	defect.State = "bogus"
	red := gateRun{Seq: 4, ExecSeq: ptr(int64(5)), Runner: ptr("r:go-test"), Location: "d:repo", Outcome: store.ExecFailed, ExitCode: ptr(1)}
	green := gateRun{Seq: 8, ExecSeq: ptr(int64(9)), Runner: ptr("r:go-test"), Location: "d:repo", Outcome: store.ExecOK}
	fix := write{Seq: 6, Path: ptr("pkg/a.go"), Class: ClassOf("pkg/a.go"), Tool: "Edit"}
	obs := obligations(obSnap(defect), []gateRun{red, green}, []write{fix})
	rowOf(t, obs, WatchedRedThenGreen) // a watched met, unless the state forces it
	for _, o := range obs {
		if o.State != ObligationUnknown {
			t.Errorf("a state outside States: %s reads %s, want unknown", o.Kind, o.State)
		}
	}
}

// TestPG3a_MaskedIsUnmetExitUnmasked: exit_unmasked is unmet when a pipe hid
// the last run's status, met where a status was read (Green, Red, Stale), and
// unknown where none was (Unknown, Backgrounded: Gate B F2). A masked run whose
// summary line reads a pass is still masked_inconclusive until PF-PG2 lands
// (gates.go, maskedReason), so it is unmet; an unmasked pass is met (C2). An
// Absent gate has no run and no exit_unmasked row.
func TestPG3a_MaskedIsUnmetExitUnmasked(t *testing.T) {
	r := runOf(t, maskedCall(1))
	obs := obAt(r, 2)
	wantRow(t, rowOf(t, obs, ExitUnmasked), ObligationUnmet, declSeq(r, 0))
	wantRow(t, rowOf(t, obs, GateAfterWrite), ObligationUnknown, declSeq(r, 0))

	r = runOf(t, withOutput(maskedCall(1), outSummaryPass))
	if snap, obs := EvaluateWithObligations(r, nil, nil, Point{Kind: PointStop, Seq: 2}, time.Time{}); len(snap.Gates) != 1 ||
		snap.Gates[0].Reason != MaskedWithSuccessLine {
		t.Fatalf("gates %+v, want one masked with a success line", snap.Gates)
	} else {
		wantRow(t, rowOf(t, obs, ExitUnmasked), ObligationUnmet, declSeq(r, 0))
	}

	r = runOf(t, withOutput(gateCall(1, "go-test", store.ExecOK), outSummaryPass))
	wantRow(t, rowOf(t, obAt(r, 2), ExitUnmasked), ObligationMet, declSeq(r, 0))

	for _, c := range []struct {
		state State
		want  ObligationState
	}{
		{Green, ObligationMet}, {Red, ObligationMet}, {Stale, ObligationMet},
		{MaskedInconclusive, ObligationUnmet}, {Unknown, ObligationUnknown}, {Backgrounded, ObligationUnknown},
	} {
		wantRow(t, rowOf(t, obligations(obSnap(obGate(c.state)), nil, nil), ExitUnmasked), c.want, 10)
	}
	noRow(t, obligations(obSnap(obGate(Absent)), nil, nil), ExitUnmasked)
}

// TestINV2_IncompleteIsUnknownNeverUnmet: with recording not complete, every
// row is unknown, whatever the gate's state, a Red one included. Evaluate
// returns no gates then, so this drives obligations() with a built snapshot;
// through EvaluateWithObligations an incomplete record's ledger is empty, and
// never null (Gate B item 3).
func TestINV2_IncompleteIsUnknownNeverUnmet(t *testing.T) {
	green := obGate(Green)
	green.Gate, green.Family = "learned:test:r:go-vet@d:repo", vetFamily
	snap := obSnap(obGate(Red), green)
	runs := []gateRun{
		{Seq: 10, ExecSeq: ptr(int64(11)), Runner: ptr("r:go-test"), Location: "d:repo", Outcome: store.ExecFailed, ExitCode: ptr(1)},
	}
	complete := obligations(snap, runs, nil)
	snap.Recording = Incomplete
	obs := obligations(snap, runs, nil)
	if len(obs) == 0 || len(obs) != len(complete) {
		t.Fatalf("%d rows incomplete, %d complete: want the same rows", len(obs), len(complete))
	}
	for _, o := range obs {
		if o.State != ObligationUnknown {
			t.Errorf("incomplete: %s reads %s, want unknown", o.Kind, o.State)
		}
	}

	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed))
	r.Skipped = 1
	s, obs := EvaluateWithObligations(r, nil, nil, Point{Kind: PointStop, Seq: 4}, time.Time{})
	if s.Recording == Complete || obs == nil || len(obs) != 0 {
		t.Fatalf("recording %s, ledger %+v: want incomplete and empty", s.Recording, obs)
	}
}

// TestPG7a_RedWriteGreenMeetsWatched: the family went red, a relevant write
// followed, then it went green: met, citing the red, the last write between
// them and the green (PF-PG7).
func TestPG7a_RedWriteGreenMeetsWatched(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go"), gateCall(3, "go-test", store.ExecOK))
	obs := obAt(r, 6)
	wantRow(t, rowOf(t, obs, WatchedRedThenGreen), ObligationMet, declSeq(r, 0), declSeq(r, 1), declSeq(r, 2))
	wantRow(t, rowOf(t, obs, GateAfterWrite), ObligationMet, declSeq(r, 2))
	if want := []ObligationKind{GateAfterWrite, WatchedRedThenGreen, ExitUnmasked, EvidenceBound}; !slices.Equal(kindsOf(obs), want) {
		t.Fatalf("kinds %v, want %v", kindsOf(obs), want)
	}

	r = runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go"), edit(3, "pkg/b.go"), gateCall(4, "go-test", store.ExecOK))
	wantRow(t, rowOf(t, obAt(r, 8), WatchedRedThenGreen), ObligationMet, declSeq(r, 0), declSeq(r, 2), declSeq(r, 3))
}

// TestPG7b_GreenOnlyDoesNotMeetWatched: a family with no red owes no watched
// row; red then green with no relevant write between is unmet, a pass by
// re-run; red with no green after it is open. The red is the last one, the
// green the first after it, and the write counts only after the red landed
// (Q4) and only when it is relevant and named. A masked failure is not a red
// (Q3), and a red in one family owes nothing in another.
func TestPG7b_GreenOnlyDoesNotMeetWatched(t *testing.T) {
	ok, failed := store.ExecOK, store.ExecFailed
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", ok))
	noRow(t, obAt(r, 4), WatchedRedThenGreen)

	r = runOf(t, gateCall(1, "go-test", failed), gateCall(2, "go-test", ok))
	wantRow(t, rowOf(t, obAt(r, 4), WatchedRedThenGreen), ObligationUnmet, declSeq(r, 0), declSeq(r, 1))

	r = runOf(t, gateCall(1, "go-test", failed), edit(2, "pkg/a.go"))
	wantRow(t, rowOf(t, obAt(r, 4), WatchedRedThenGreen), ObligationOpen, declSeq(r, 0))

	r = runOf(t, gateCall(1, "go-test", failed), edit(2, "pkg/a.go"), gateCall(3, "go-test", ok))
	wantRow(t, rowOf(t, obAt(r, 5), WatchedRedThenGreen), ObligationOpen, declSeq(r, 0)) // the green has not landed

	r = runOf(t, gateCall(1, "go-test", failed), edit(2, "README.md"), unnamedWrite(3), gateCall(4, "go-test", ok))
	wantRow(t, rowOf(t, obAt(r, 8), WatchedRedThenGreen), ObligationUnmet, declSeq(r, 0), declSeq(r, 3))

	// The last red: a fix, then a flake passed by re-run (Q5).
	r = runOf(t, gateCall(1, "go-test", failed), edit(2, "pkg/a.go"), gateCall(3, "go-test", ok),
		gateCall(4, "go-test", failed), gateCall(5, "go-test", ok))
	wantRow(t, rowOf(t, obAt(r, 10), WatchedRedThenGreen), ObligationUnmet, declSeq(r, 3), declSeq(r, 4))

	// A green before the red is not after it, nor one before the last red.
	r = runOf(t, gateCall(1, "go-test", ok), edit(2, "pkg/a.go"), gateCall(3, "go-test", failed))
	wantRow(t, rowOf(t, obAt(r, 6), WatchedRedThenGreen), ObligationOpen, declSeq(r, 2))
	r = runOf(t, gateCall(1, "go-test", failed), edit(2, "pkg/a.go"), gateCall(3, "go-test", ok), gateCall(4, "go-test", failed))
	wantRow(t, rowOf(t, obAt(r, 8), WatchedRedThenGreen), ObligationOpen, declSeq(r, 3))

	masked := maskedCall(1)
	masked.outcome, masked.exit = failed, 1
	r = runOf(t, masked, edit(2, "pkg/a.go"), gateCall(3, "go-test", ok))
	noRow(t, obAt(r, 6), WatchedRedThenGreen)

	r = runOf(t, gateCall(1, "go-test", failed), edit(2, "pkg/a.go"), gateCall(3, "go-test", ok), gateCall(4, "go-vet", ok))
	obs := obAt(r, 8)
	if n := len(slices.DeleteFunc(slices.Clone(obs), func(o Obligation) bool { return o.Kind != WatchedRedThenGreen })); n != 1 {
		t.Fatalf("%d watched rows over two families with one red, want one: %+v", n, obs)
	}

	// In flight (Q4): the write at 11 was declared before the red's execution
	// landed at 12, so it is not a response to the red; the one at 13 is.
	red := gateRun{Seq: 10, ExecSeq: ptr(int64(12)), Runner: ptr("r:go-test"), Location: "d:repo", Outcome: failed, ExitCode: ptr(1)}
	green := gateRun{Seq: 14, ExecSeq: ptr(int64(15)), Runner: ptr("r:go-test"), Location: "d:repo", Outcome: ok}
	inFlight := write{Seq: 11, Path: ptr("pkg/a.go"), Class: ClassOf("pkg/a.go"), Tool: "Edit"}
	after := write{Seq: 13, Path: ptr("pkg/b.go"), Class: ClassOf("pkg/b.go"), Tool: "Edit"}
	snap := obSnap(obGate(Green))
	wantRow(t, rowOf(t, obligations(snap, []gateRun{red, green}, []write{inFlight}), WatchedRedThenGreen), ObligationUnmet, 10, 14)
	wantRow(t, rowOf(t, obligations(snap, []gateRun{red, green}, []write{inFlight, after}), WatchedRedThenGreen), ObligationMet, 10, 13, 14)
}

// TestPG5c_LedgerIsByteIdenticalOverShuffledRecords: the ledger depends on no
// record order and no map order (PF-PG5 done-when; the VER44 method).
func TestPG5c_LedgerIsByteIdenticalOverShuffledRecords(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go"), gateCall(3, "go-vet", store.ExecFailed),
		gateCall(4, "go-test", store.ExecOK), bashWrite(5, "pkg/b.go"), gateCall(6, "go-vet", store.ExecOK), edit(7, "pkg/c.go"))
	want := encodeObs(t, obAt(r, 14))
	if obs := obAt(r, 14); len(obs) != 8 {
		t.Fatalf("%d rows, want eight: the fixture must carry two families with every kind", len(obs))
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(r.Declarations), func(i, j int) { r.Declarations[i], r.Declarations[j] = r.Declarations[j], r.Declarations[i] })
		rng.Shuffle(len(r.Executions), func(i, j int) { r.Executions[i], r.Executions[j] = r.Executions[j], r.Executions[i] })
		if got := encodeObs(t, obAt(r, 14)); !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d:\n%s\nwant\n%s", i, got, want)
		}
	}
}

// TestPG5d_IDIsAStableDigestOfScopeKindFamily: an id is "ob:" and the first 16
// hex of sha256 over scope, kind and family joined by newlines. It is the
// same across evaluations and across a gate label change (D33 item 3), and
// differs across each of its three inputs.
func TestPG5d_IDIsAStableDigestOfScopeKindFamily(t *testing.T) {
	sum := sha256.Sum256([]byte("session\ngate_after_write\n" + testFamily))
	want := "ob:" + hex.EncodeToString(sum[:])[:16]
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	if got := rowOf(t, obAt(r, 4), GateAfterWrite).ID; got != want {
		t.Fatalf("id %q, want %q", got, want)
	}
	if a, b := encodeObs(t, obAt(r, 4)), encodeObs(t, obAt(r, 4)); !bytes.Equal(a, b) {
		t.Fatalf("two evaluations differ:\n%s\n%s", a, b)
	}

	relabelled := obGate(Green)
	relabelled.Gate = "learned:build:r:go-test@d:repo"
	if a, b := obligations(obSnap(obGate(Green)), nil, nil), obligations(obSnap(relabelled), nil, nil); a[0].ID != b[0].ID {
		t.Fatalf("a label change moved the id: %q and %q", a[0].ID, b[0].ID)
	}

	base := ObligationID(ScopeSession, GateAfterWrite, testFamily)
	for _, other := range []string{
		ObligationID(ScopeCommit, GateAfterWrite, testFamily),
		ObligationID(ScopeSession, ExitUnmasked, testFamily),
		ObligationID(ScopeSession, GateAfterWrite, vetFamily),
	} {
		if other == base {
			t.Errorf("id %q does not change with its input", base)
		}
	}

	r = runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go"), gateCall(3, "go-test", store.ExecOK), gateCall(4, "go-vet", store.ExecOK))
	seen := map[string]bool{}
	for _, o := range obAt(r, 8) {
		if seen[o.ID] {
			t.Errorf("id %q twice", o.ID)
		}
		seen[o.ID] = true
	}
}

// TestPG5e_EveryListNonNilAndOnlyV0Kinds: the ledger and every Evidence encode
// as lists; v0 emits only its four kinds, per required gate in snapshot order
// and per gate in kind order; the scope is the session at every point kind
// (Q1); the source is spec or learned, and any other leaves the gate's rows
// unknown (A1); and the ledger at a point equals the ledger of the record's
// prefix at that point (INV-11), but for RES14c's exception, which moves rows
// and nothing else.
func TestPG5e_EveryListNonNilAndOnlyV0Kinds(t *testing.T) {
	if b := encodeObs(t, obligations(Snapshot{}, nil, nil)); string(b) != "[]" {
		t.Fatalf("an empty ledger encodes %s, want []", b)
	}
	if want := []ObligationKind{GateAfterWrite, WatchedRedThenGreen, ExitUnmasked, EvidenceBound,
		SpecRule, PlatformImpact, ExpectedEvidence, ProposedCheck}; !slices.Equal(ObligationKinds, want) {
		t.Fatalf("ObligationKinds %v, want %v", ObligationKinds, want)
	}

	full := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/a.go"), gateCall(3, "go-vet", store.ExecOK),
		gateCall(4, "go-test", store.ExecOK), edit(5, "pkg/b.go"))
	obs := obAt(full, 10)
	b := encodeObs(t, obs)
	if bytes.Contains(b, []byte(`"evidence":null`)) || bytes.Count(b, []byte(`"evidence":[`)) != len(obs) {
		t.Fatalf("an evidence list is not a list: %s", b)
	}
	v0 := ObligationKinds[:4]
	var families []string
	for _, o := range obs {
		if !slices.Contains(v0, o.Kind) {
			t.Errorf("a %s row in v0", o.Kind)
		}
	}
	snap, _ := EvaluateWithObligations(full, nil, nil, Point{Kind: PointStop, Seq: 10}, time.Time{})
	for _, g := range snap.Gates {
		families = append(families, g.Family)
	}
	var want []string
	for _, f := range families {
		for _, k := range v0 {
			if k != WatchedRedThenGreen || f == testFamily {
				want = append(want, ObligationID(ScopeSession, k, f))
			}
		}
	}
	var got []string
	for _, o := range obs {
		got = append(got, o.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows out of order: ids %v, want %v (families %v)", got, want, families)
	}

	for _, k := range PointKinds {
		snap := obSnap(obGate(Green))
		snap.Point.Kind = k
		for _, o := range obligations(snap, nil, nil) {
			if o.Scope != ScopeSession {
				t.Errorf("point %s: scope %q, want session", k, o.Scope)
			}
		}
	}

	for _, c := range []struct{ source, want string }{
		{"declared", "spec"}, {"ci_commands", "spec"}, {sourceLearned, sourceLearned}, {"bogus", "bogus"},
	} {
		g := obGate(Green)
		g.Source = c.source
		for _, o := range obligations(obSnap(g), nil, nil) {
			if o.Source != c.want {
				t.Errorf("source %q reads %q, want %q", c.source, o.Source, c.want)
			}
			if c.source == "bogus" && o.State != ObligationUnknown {
				t.Errorf("source %q: %s reads %s, want unknown", c.source, o.Kind, o.State)
			}
		}
	}

	for s := int64(0); s <= 10; s++ {
		if a, b := encodeObs(t, obAt(full, s)), encodeObs(t, obAt(prefixOf(full, s), s)); !bytes.Equal(a, b) {
			t.Fatalf("at %d the ledger differs from the prefix's:\n%s\n%s", s, a, b)
		}
	}

	// RES14c's exception reaches the ledger as order only. A runner whose
	// recorded kind lands after the point names its gate by that kind at the
	// point and by its inferred kind in the prefix; a gate whose label sorts
	// between the two moves (build:r:make-ci < build:r:npm-build at the point,
	// build:r:npm-build < custom:r:make-ci in the prefix). Sorted by id, the two
	// ledgers are equal. If the order is ever made stable, drop the first
	// check and the doc's exception with it.
	v5 := call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-ci", masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo", schema: 5}
	npm := call{seq: 2, tool: "Bash", prog: "npm", class: shape.VerbExecute, runner: "npm-build", kind: shape.RunnerBuild, masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo"}
	v6 := v5
	v6.seq, v6.kind, v6.schema = 3, shape.RunnerBuild, 0
	relabel := runOf(t, v5, npm, v6)
	cut, prefix := obAt(relabel, 4), obAt(prefixOf(relabel, 4), 4)
	if bytes.Equal(encodeObs(t, cut), encodeObs(t, prefix)) {
		t.Fatalf("the fixture moves no row; it must, to show the exception is order only:\n%s", encodeObs(t, cut))
	}
	byID := func(a, b Obligation) int { return cmp.Compare(a.ID, b.ID) }
	slices.SortFunc(cut, byID)
	slices.SortFunc(prefix, byID)
	if a, b := encodeObs(t, cut), encodeObs(t, prefix); !bytes.Equal(a, b) {
		t.Fatalf("beyond the order the point cut's ledger differs from the prefix's:\n%s\n%s", a, b)
	}
}
