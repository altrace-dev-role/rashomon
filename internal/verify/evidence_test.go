package verify

import (
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// evSpec is a compiled spec with a task naming kinds and mechanism, and
// gates.
func evSpec(kinds []string, mechanism string, gates ...spec.Declared) *spec.Compiled {
	return &spec.Compiled{Task: &spec.Task{ID: "t1", Goal: "g", Decision: "d", ExpectedEvidence: kinds,
		Mechanism: mechanism}, Gates: gates}
}

// supplier is the declared gate spec:<id> whose run supplies kinds.
func supplier(id string, kinds ...string) spec.Declared {
	return spec.Declared{ID: "spec:" + id, Gate: spec.Gate{ID: id, Kind: "test", Argv: []string{"go", "test"},
		Supplies: kinds}}
}

func evSnap(gates ...GateState) Snapshot { return Snapshot{Recording: Complete, Gates: gates} }

func gateIn(id string, s State) GateState {
	return GateState{Gate: "spec:" + id, State: s, Required: true}
}

func oneRow(t *testing.T, ex []Expected) Expected {
	t.Helper()
	if len(ex) != 1 {
		t.Fatalf("rows %+v, want one", ex)
	}
	return ex[0]
}

// TestEV1_DeclaredEvidenceRunMeetsTheKind: a run kind reads gate_after_write
// over the declared gates whose supplies carry it: green after the last write
// is met, red or stale is unmet naming the gate; no supplying gate is open
// before Stop and unmet at it, "no gate declares it"; a gate supplying
// another kind supplies nothing here; at Stop a supplying gate with a write
// and no run is unmet; any supplying gate met is enough.
func TestEV1_DeclaredEvidenceRunMeetsTheKind(t *testing.T) {
	rdr := []string{"real_data_run"}
	for _, c := range []struct {
		name   string
		sp     *spec.Compiled
		snap   Snapshot
		atStop bool
		want   Expected
	}{
		{"green", evSpec(rdr, "", supplier("r", "real_data_run")), evSnap(gateIn("r", Green)), true,
			Expected{Kind: "real_data_run", State: ObligationMet, Gate: "spec:r"}},
		{"red", evSpec(rdr, "", supplier("r", "real_data_run")), evSnap(gateIn("r", Red)), false,
			Expected{Kind: "real_data_run", State: ObligationUnmet, Gate: "spec:r", Reason: "no green run of spec:r since the last edit"}},
		{"stale", evSpec(rdr, "", supplier("r", "real_data_run")), evSnap(gateIn("r", Stale)), true,
			Expected{Kind: "real_data_run", State: ObligationUnmet, Gate: "spec:r", Reason: "no green run of spec:r since the last edit"}},
		{"no supplier before Stop", evSpec(rdr, ""), evSnap(), false,
			Expected{Kind: "real_data_run", State: ObligationOpen}},
		{"no supplier at Stop", evSpec(rdr, ""), evSnap(), true,
			Expected{Kind: "real_data_run", State: ObligationUnmet, Reason: "no gate declares it"}},
		{"another kind's supplier", evSpec(rdr, "", supplier("h", "hostile_input_run")), evSnap(gateIn("h", Green)), true,
			Expected{Kind: "real_data_run", State: ObligationUnmet, Reason: "no gate declares it"}},
		{"written, no run, before Stop", evSpec(rdr, "", supplier("r", "real_data_run")), evSnap(gateIn("r", Absent)), false,
			Expected{Kind: "real_data_run", State: ObligationOpen, Gate: "spec:r"}},
		{"written, no run, at Stop", evSpec(rdr, "", supplier("r", "real_data_run")), evSnap(gateIn("r", Absent)), true,
			Expected{Kind: "real_data_run", State: ObligationUnmet, Gate: "spec:r", Reason: "no green run of spec:r since the last edit"}},
		{"second supplier green", evSpec(rdr, "", supplier("a", "real_data_run"), supplier("b", "real_data_run")),
			evSnap(gateIn("a", Red), gateIn("b", Green)), true,
			Expected{Kind: "real_data_run", State: ObligationMet, Gate: "spec:b"}},
	} {
		if got := oneRow(t, expected(c.snap, evidenceIn{sp: c.sp, atStop: c.atStop, wrote: true})); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestEV1b_RowsInEvidenceKindsOrder: the rows follow spec.EvidenceKinds,
// whatever order the task file lists them in, and only the kinds it lists.
func TestEV1b_RowsInEvidenceKindsOrder(t *testing.T) {
	sp := evSpec([]string{"mechanism_stated", "judgment_disclosed", "cross_check", "real_data_run"}, "")
	ex := expected(evSnap(), evidenceIn{sp: sp})
	want := []string{"real_data_run", "cross_check", "judgment_disclosed", "mechanism_stated"}
	if len(ex) != len(want) {
		t.Fatalf("rows %+v, want %v", ex, want)
	}
	for i, k := range want {
		if ex[i].Kind != k {
			t.Errorf("row %d is %s, want %s (rows %+v)", i, ex[i].Kind, k, ex)
		}
	}
}

// TestEV1c_IncompleteRecordingReadsUnknown: an incomplete record reads every
// row unknown, with no reason, and UnmetExpected names none of them (INV-2).
func TestEV1c_IncompleteRecordingReadsUnknown(t *testing.T) {
	snap := evSnap(gateIn("r", Red))
	snap.Recording = Incomplete
	ex := expected(snap, evidenceIn{sp: evSpec([]string{"real_data_run", "mechanism_stated"}, "", supplier("r", "real_data_run")),
		atStop: true, wrote: true})
	if len(ex) != 2 {
		t.Fatalf("%d rows, want one per expected kind (2): an incomplete record keeps its rows, as unknown", len(ex))
	}
	for _, e := range ex {
		if e.State != ObligationUnknown || e.Reason != "" {
			t.Errorf("%+v, want unknown with no reason", e)
		}
	}
	if u := UnmetExpected(ex); len(u) != 0 {
		t.Errorf("UnmetExpected over unknown rows: %+v, want none", u)
	}
}

// TestEV2_NoMechanismAtFirstWriteIsUnmet: an empty mechanism is open before
// the first relevant write and unmet from it, "the task file states no
// mechanism".
func TestEV2_NoMechanismAtFirstWriteIsUnmet(t *testing.T) {
	sp := evSpec([]string{"mechanism_stated"}, "")
	if got := oneRow(t, expected(evSnap(), evidenceIn{sp: sp})); got.State != ObligationOpen {
		t.Errorf("before any write: %+v, want open", got)
	}
	want := Expected{Kind: "mechanism_stated", State: ObligationUnmet, Reason: "the task file states no mechanism"}
	if got := oneRow(t, expected(evSnap(), evidenceIn{sp: sp, wrote: true})); got != want {
		t.Errorf("after a write: %+v, want %+v", got, want)
	}
}

// TestEV3_HumanLineMeetsMechanism: a stated mechanism is met after a write
// when no tool call targeted the task file; a call that did (3.2's
// StoreTouched entry, outcome-blind) makes it unmet with a reason that says
// "targeted", never "wrote" (D135); a call on another store file does not.
func TestEV3_HumanLineMeetsMechanism(t *testing.T) {
	sp := evSpec([]string{"mechanism_stated"}, "the cache is keyed by path and size")
	in := evidenceIn{sp: sp, wrote: true}
	if got := oneRow(t, expected(evSnap(), in)); got.State != ObligationMet {
		t.Errorf("a person's mechanism: %+v, want met", got)
	}
	touched := func(path string) Snapshot {
		s := evSnap()
		s.Integrity.Unresolved = []IntegrityEntry{{Check: StoreTouched, Path: path, Seq: 4}}
		return s
	}
	want := Expected{Kind: "mechanism_stated", State: ObligationUnmet, Reason: "a tool call targeted the task file"}
	if got := oneRow(t, expected(touched(".claude/rashomon/task/current.json"), in)); got != want {
		t.Errorf("a call that targeted the task file: %+v, want %+v", got, want)
	}
	if got := oneRow(t, expected(touched(".claude/rashomon/spec.json"), in)); got.State != ObligationMet {
		t.Errorf("a call that targeted the spec, not the task: %+v, want met", got)
	}
}

// TestEV4_JudgmentDisclosedReadsTheFinalMessage: a final message with a
// disclosed sentence meets judgment_disclosed at Stop; one without is unmet;
// before Stop it is open. Claims sets Disclosed by the sentence reader's own
// disclosed(), not a word list of its own: "no errors" is an accepted
// predicate's words, which disclose nothing.
func TestEV4_JudgmentDisclosedReadsTheFinalMessage(t *testing.T) {
	disc := Claims([]byte("The parser is done. I did not run it against the production export."), nil, []byte("k"), nil, true, SourceFinalMessage, nil)
	plain := Claims([]byte("The parser is done. It reads every row of the sample file."), nil, []byte("k"), nil, true, SourceFinalMessage, nil)
	if !disc.Disclosed || plain.Disclosed {
		t.Fatalf("Disclosed: %v for a disclosing message, %v for a plain one; want true, false", disc.Disclosed, plain.Disclosed)
	}
	sp := evSpec([]string{"judgment_disclosed"}, "")
	if got := oneRow(t, expected(evSnap(), evidenceIn{sp: sp, atStop: true, disclosed: disc.Disclosed})); got.State != ObligationMet {
		t.Errorf("disclosed at Stop: %+v, want met", got)
	}
	want := Expected{Kind: "judgment_disclosed", State: ObligationUnmet, Reason: "the final message discloses no judgment"}
	if got := oneRow(t, expected(evSnap(), evidenceIn{sp: sp, atStop: true, disclosed: plain.Disclosed})); got != want {
		t.Errorf("not disclosed at Stop: %+v, want %+v", got, want)
	}
	if got := oneRow(t, expected(evSnap(), evidenceIn{sp: sp})); got.State != ObligationOpen {
		t.Errorf("before Stop: %+v, want open", got)
	}
}

// TestEV5_NoTaskNoRows: no spec, or a spec with no task, owes no expected
// row (an empty list, never nil); with a task, each row is one
// expected_evidence obligation from the task, its id over the kind.
func TestEV5_NoTaskNoRows(t *testing.T) {
	for _, sp := range []*spec.Compiled{nil, {}} {
		if ex := expected(evSnap(), evidenceIn{sp: sp, atStop: true, wrote: true}); ex == nil || len(ex) != 0 {
			t.Errorf("spec %+v: %+v, want no rows", sp, ex)
		}
	}
	ex := expected(evSnap(), evidenceIn{sp: evSpec([]string{"cross_check"}, ""), atStop: true})
	obs := expectedObligations(ScopeSession, evSnap(), ex)
	if len(obs) != 1 || obs[0].Kind != ExpectedEvidence || obs[0].Source != SourceTask || obs[0].State != ObligationUnmet ||
		len(obs[0].Evidence) != 0 || obs[0].Evidence == nil || obs[0].ID != ObligationID(ScopeSession, ExpectedEvidence, "cross_check") {
		t.Errorf("obligations %+v, want one unmet expected_evidence row from the task over cross_check", obs)
	}
}

// TestEV8_MetRunRowCitesTheSupplyingGreen (card 3.3b, D143): a met run row
// cites the supplying gate's last green run, read from the snapshot when the
// ledger is built; a met text row, the same gate open, and the gate stale
// after that green cite nothing, as an empty list and never null.
func TestEV8_MetRunRowCitesTheSupplyingGreen(t *testing.T) {
	empty := func(ev []int64) bool { return len(ev) == 0 && ev != nil }
	green := gateIn("r", Green)
	green.LastGreen = &RunRef{Seq: 7}
	sp := evSpec([]string{"real_data_run", "mechanism_stated"}, "the cache is keyed by path and size", supplier("r", "real_data_run"))
	obs := expectedObligations(ScopeSession, evSnap(green), expected(evSnap(green), evidenceIn{sp: sp, atStop: true, wrote: true}))
	if len(obs) != 2 || obs[0].State != ObligationMet || !slices.Equal(obs[0].Evidence, []int64{7}) ||
		obs[1].State != ObligationMet || !empty(obs[1].Evidence) {
		t.Fatalf("rows %+v; want the met run row citing [7] and the met text row citing nothing", obs)
	}
	rdr := evSpec([]string{"real_data_run"}, "", supplier("r", "real_data_run"))
	stale := gateIn("r", Stale)
	stale.LastGreen = &RunRef{Seq: 7}
	for _, c := range []struct {
		name  string
		g     GateState
		state ObligationState
	}{{"open", gateIn("r", Absent), ObligationOpen}, {"stale after its green", stale, ObligationUnmet}} {
		obs := expectedObligations(ScopeSession, evSnap(c.g), expected(evSnap(c.g), evidenceIn{sp: rdr}))
		if len(obs) != 1 || obs[0].State != c.state || !empty(obs[0].Evidence) {
			t.Errorf("%s: %+v, want %s citing an empty, non-nil list", c.name, obs, c.state)
		}
	}
}

// TestEV5b_StopAndLedgerCarryTheRows: EvaluateStop's view and the ledger
// both carry the task's rows: one Expected per kind at Stop, and one
// expected_evidence obligation per kind beside the gates' rows.
func TestEV5b_StopAndLedgerCarryTheRows(t *testing.T) {
	sp := evSpec([]string{"cross_check", "judgment_disclosed"}, "")
	pt := Point{Kind: PointStop, Seq: 1, SessionID: "s"}
	if v := EvaluateStop(&store.Run{}, nil, sp, pt, time.UnixMilli(1), StopInput{}); len(v.Expected) != 2 {
		t.Errorf("StopView.Expected = %+v, want the task's two kinds", v.Expected)
	}
	_, obs := EvaluateWithObligations(&store.Run{}, nil, sp, pt, time.UnixMilli(1))
	n := 0
	for _, o := range obs {
		if o.Kind == ExpectedEvidence {
			n++
		}
	}
	if n != 2 {
		t.Errorf("the ledger holds %d expected_evidence rows, want 2: %+v", n, obs)
	}
}
