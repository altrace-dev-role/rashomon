package verify

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Seqs below are the builder's counter: with no terminal, call i has
// declaration 2i-1 and execution 2i; an ordered terminal takes the Seq
// between them, and a lost declaration takes none.

// wantRecording fails the test unless the run's recording at the point at is
// rec with the notes want, in that order; with no notes, Complete's nil, not
// an empty list.
func wantRecording(t *testing.T, r *store.Run, at int64, rec Recording, want ...string) {
	t.Helper()
	got, gotNotes := recording(r, at)
	if got != rec || !slices.Equal(gotNotes, want) || (gotNotes == nil) != (want == nil) {
		t.Fatalf("at %d: %s %#v, want %s %#v", at, got, gotNotes, rec, want)
	}
}

// dropExecution and dropDeclaration remove one call's record of that kind,
// as a store that lost it would read back.
func dropExecution(r *store.Run, id string) {
	r.Executions = slices.DeleteFunc(r.Executions, func(e store.Execution) bool { return e.ToolUseID == id })
}

func dropDeclaration(r *store.Run, id string) {
	r.Declarations = slices.DeleteFunc(r.Declarations, func(d store.Declaration) bool { return d.ToolUseID == id })
}

// lost is c with its declaration never written: an ordered execution and the
// spilled lock_timeout terminal Handler.Close writes in its place.
func lost(c call) call {
	c.lostDeclaration, c.term = true, store.OutcomeError
	return c
}

// verifiedAt is a verified record of the phase, its entry and probe resolved.
func verifiedAt(phase string) store.Coverage {
	c := postCoverage(store.StateVerified, nil, store.EntryPresentSettings, store.ProbeFresh)
	c.Phase = phase
	return c
}

func TestVER30_CompleteWhenEveryDeclarationHasAnOutcome(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	wantRecording(t, r, 4, Complete)
}

func TestVER31_SkippedLinesAreIncomplete(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	r.Skipped = 1
	wantRecording(t, r, 4, Incomplete, "skipped_lines")

	// An ignored record is a non-critical one an older reader passes over.
	r.Skipped, r.Ignored = 0, 1
	wantRecording(t, r, 4, Complete)
}

func TestVER32_DeclarationWithoutOutcome(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	dropExecution(r, "t2")
	wantRecording(t, r, 4, Incomplete, "declaration_without_outcome")
	// Its declaration (3) is after the point, so no outcome is due there.
	wantRecording(t, r, 2, Complete)

	// An ordered terminal answers the declaration on its own (t2: declaration
	// 3, terminal 4, execution 5).
	term := gateCall(2, "go-test", store.ExecOK)
	term.term = store.OutcomeOK
	r = runOf(t, edit(1, "pkg/a.go"), term)
	dropExecution(r, "t2")
	wantRecording(t, r, 5, Complete)
}

// An outcome is a fact of the record, wherever it landed: whether a run
// stands at the point is gateStates' to say, not this function's.
func TestVER32b_InFlightIsNotAGap(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	wantRecording(t, r, 3, Complete)
}

// An outcome whose declaration the record lacks hides what the call was: a
// lost Edit is a write gateStates never sees (D35).
func TestVER32c_OutcomeWithoutDeclaration(t *testing.T) {
	// t2 has no declaration, its execution is 3, and its terminal is spilled.
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), lost(edit(2, "pkg/a.go")))
	wantRecording(t, r, 3, Incomplete, "outcome_without_declaration")
	// The spilled terminal cannot be placed, so it counts at every point.
	wantRecording(t, r, 2, Incomplete, "outcome_without_declaration")

	// An orphan execution (4) counts from where it landed.
	r = runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	dropDeclaration(r, "t2")
	wantRecording(t, r, 3, Complete)
	wantRecording(t, r, 4, Incomplete, "outcome_without_declaration")

	// So does an ordered orphan terminal (2): no writer leaves one, and one
	// rule serves both outcomes.
	term := gateCall(1, "go-test", store.ExecOK)
	term.term = store.OutcomeOK
	r = runOf(t, term)
	dropDeclaration(r, "t1")
	dropExecution(r, "t1")
	wantRecording(t, r, 1, Complete)
	wantRecording(t, r, 2, Incomplete, "outcome_without_declaration")
}

func TestVER33_SpilledOutcomeIsIncomplete(t *testing.T) {
	r := runOf(t, spilledGate(1, store.ExecOK))
	wantRecording(t, r, 1, Incomplete, "spilled_outcome")

	// A spilled call declared after the point (3) is not due there.
	r = runOf(t, edit(1, "pkg/a.go"), spilledGate(2, store.ExecOK))
	wantRecording(t, r, 2, Complete)
	wantRecording(t, r, 3, Incomplete, "spilled_outcome")

	// Either outcome spilled alone is the fault (t1: declaration 1, terminal
	// 2, execution 3).
	term := gateCall(1, "go-test", store.ExecOK)
	term.term = store.OutcomeOK
	r = runOf(t, term)
	r.Executions[0].Seq = nil
	wantRecording(t, r, 3, Incomplete, "spilled_outcome")
	r = runOf(t, term)
	r.Terminals[0].Seq = nil
	wantRecording(t, r, 3, Incomplete, "spilled_outcome")
}

func TestVER34_Coverage(t *testing.T) {
	verified := verifiedAt(store.PhasePost)
	unknownProbe := postCoverage(store.StateUnverified, strp(store.ReasonProbeUnresolved), store.EntryPresentSettings, store.ProbeUnknown)
	for _, tc := range []struct {
		name string
		cov  []store.Coverage
		want []string
	}{
		{"the builder's default", nil, nil},
		{"unverified with its reason", []store.Coverage{postCoverage(store.StateUnverified, strp(store.ReasonHookEntryAbsent), store.EntryAbsent, store.ProbeFresh)}, []string{"coverage_unverified"}},
		{"unverified with no reason", []store.Coverage{postCoverage(store.StateUnverified, nil, store.EntryPresentSettings, store.ProbeFresh)}, []string{"coverage_unverified"}},
		{"verified with the probe unknown", []store.Coverage{postCoverage(store.StateVerified, nil, store.EntryPresentSettings, store.ProbeUnknown)}, []string{"coverage_unverified"}},
		{"verified with the entry unknown", []store.Coverage{postCoverage(store.StateVerified, nil, store.EntryUnknown, store.ProbeFresh)}, []string{"coverage_unverified"}},
		{"verified with a reason", []store.Coverage{postCoverage(store.StateVerified, strp(store.ReasonHookEntryAbsent), store.EntryPresentSettings, store.ProbeFresh)}, []string{"coverage_unverified"}},
		{"a zero State", []store.Coverage{postCoverage("", nil, store.EntryPresentSettings, store.ProbeFresh)}, []string{"coverage_unverified"}},
		{"one verified and one entry absent", []store.Coverage{verified, postCoverage(store.StateUnverified, strp(store.ReasonHookEntryAbsent), store.EntryAbsent, store.ProbeFresh)}, nil},
		{"one verified and one probe unknown", []store.Coverage{verified, unknownProbe}, []string{"coverage_unverified"}},
		{"only a start record", []store.Coverage{verifiedAt(store.PhaseStart)}, []string{"coverage_unverified"}},
		{"start, call and end records", []store.Coverage{verifiedAt(store.PhaseStart), verifiedAt(store.PhaseCall), verifiedAt(store.PhaseEnd)}, []string{"coverage_unverified"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
			if tc.cov != nil {
				r.Coverage = tc.cov
			}
			if tc.want == nil {
				wantRecording(t, r, 4, Complete)
				return
			}
			wantRecording(t, r, 4, Incomplete, tc.want...)
		})
	}

	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	r.Coverage = nil
	wantRecording(t, r, 4, Incomplete, "coverage_absent")
}

func TestVER35_NoteOrderAndTheClosedTable(t *testing.T) {
	// t2's execution is gone; t3 has no declaration, its execution is 5.
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), lost(edit(3, "pkg/b.go")))
	dropExecution(r, "t2")
	r.Skipped = 1
	r.Coverage = []store.Coverage{}
	wantRecording(t, r, 5, Incomplete, "skipped_lines", "declaration_without_outcome", "outcome_without_declaration", "coverage_absent")

	// Every note recording gives is one of these, in this order.
	want := [...]string{"skipped_lines", "declaration_without_outcome", "outcome_without_declaration", "spilled_outcome", "coverage_unverified", "coverage_absent"}
	if notes != want {
		t.Fatalf("notes %q, want %q", notes, want)
	}
}

func TestVER36_PureAndOrderIndependent(t *testing.T) {
	// t2's execution is gone, t3 lost its declaration, t4 is spilled, and one
	// post record resolved its probe to unknown.
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), lost(edit(3, "pkg/b.go")),
		spilledGate(4, store.ExecOK), edit(5, "pkg/c.go"))
	dropExecution(r, "t2")
	r.Coverage = []store.Coverage{verifiedAt(store.PhaseStart), verifiedAt(store.PhasePost),
		postCoverage(store.StateUnverified, strp(store.ReasonProbeUnresolved), store.EntryPresentSettings, store.ProbeUnknown), verifiedAt(store.PhaseEnd)}
	const at = 8
	wantRecording(t, r, at, Incomplete, "declaration_without_outcome", "outcome_without_declaration", "spilled_outcome", "coverage_unverified")

	encode := func() []byte {
		rec, ns := recording(r, at)
		b, err := json.Marshal([2]any{rec, ns})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	want := encode()
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(r.Declarations), func(i, j int) { r.Declarations[i], r.Declarations[j] = r.Declarations[j], r.Declarations[i] })
		rng.Shuffle(len(r.Executions), func(i, j int) { r.Executions[i], r.Executions[j] = r.Executions[j], r.Executions[i] })
		rng.Shuffle(len(r.Terminals), func(i, j int) { r.Terminals[i], r.Terminals[j] = r.Terminals[j], r.Terminals[i] })
		rng.Shuffle(len(r.Coverage), func(i, j int) { r.Coverage[i], r.Coverage[j] = r.Coverage[j], r.Coverage[i] })
		if got := encode(); !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d: %s, want %s", i, got, want)
		}
	}
}

func BenchmarkRecording(b *testing.B) {
	// Top-level, as BenchmarkGateStates is: TestH113 runs internal/secretshape's
	// entropy test over test/perf/budgets.json, where a sub-benchmark's name
	// can read as a secret.
	calls := make([]call, 0, 200)
	for i := range 200 {
		c := edit(int64(i+1), "pkg/f"+strconv.Itoa(i)+".go")
		if i%20 == 0 {
			c.term = store.OutcomeOK
		}
		calls = append(calls, c)
	}
	r := runOf(b, calls...)
	r.Coverage = append(r.Coverage, verifiedAt(store.PhaseStart), verifiedAt(store.PhaseCall), verifiedAt(store.PhaseEnd))
	at := *r.Executions[len(r.Executions)-1].Seq
	if len(r.Declarations) != 200 || len(r.Executions) != 200 || len(r.Terminals) != 10 || len(r.Coverage) != 4 {
		b.Fatalf("the fixture has %d declarations, %d executions, %d terminals and %d coverage records, want 200, 200, 10 and 4",
			len(r.Declarations), len(r.Executions), len(r.Terminals), len(r.Coverage))
	}
	if rec, ns := recording(r, at); rec != Complete || ns != nil {
		b.Fatalf("the fixture is %s %q, want complete", rec, ns)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		recording(r, at)
	}
}
