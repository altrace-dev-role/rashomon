package verify

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The ledger's turn rows (W-M3.1).

// inTurns sets the prompt id of r's declarations, in call order, to ids; ""
// leaves a declaration with none.
func inTurns(r *store.Run, ids ...string) *store.Run {
	for i := range ids {
		if ids[i] != "" {
			r.Declarations[i].PromptID = &ids[i]
		}
	}
	return r
}

// dirUnit is a UnitOf whose unit is a path's directory.
func dirUnit(rel string) string { return "g:r#" + path.Dir(rel) }

// turnView is the Stop view at seq at over r in promptID's turn, with unitOf
// and tree, the point's.
func turnView(r *store.Run, at int64, promptID string, unitOf UnitOf, tree map[string]*string) StopView {
	pt := Point{Kind: PointStop, Seq: at, SessionID: "s1", Tree: tree}
	return EvaluateStop(r, nil, nil, pt, stopNow, StopInput{PromptID: promptID, UnitOf: unitOf})
}

// turnOf is obs's turn rows, and of those the ones about a unit.
func turnOf(obs []Obligation) (turn, units []Obligation) {
	for _, o := range obs {
		if o.Scope != ScopeTurn {
			continue
		}
		turn = append(turn, o)
		if o.Unit != "" {
			units = append(units, o)
		}
	}
	return turn, units
}

// stopBefore is the seq just before r's i-th declaration: a Stop that ends
// the calls before it.
func stopBefore(r *store.Run, i int) int64 { return r.Declarations[i].Seq - 1 }

// promptAt is the prompt id of r's last declaration at or before s that
// carries one, "" for none: the id a Stop at s records.
func promptAt(r *store.Run, s int64) string {
	p := ""
	for _, d := range r.Declarations {
		if d.Seq <= s && d.PromptID != nil {
			p = *d.PromptID
		}
	}
	return p
}

// TestLedger_TurnRowIDsDifferAcrossTurns: two turns' rows for one gate never
// share an id. Witness: the gate's session rows at the two Stops do.
func TestLedger_TurnRowIDsDifferAcrossTurns(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/a.go"),
		gateCall(4, "go-test", store.ExecOK)), "p1", "p1", "p2", "p2")
	one, two := turnView(r, stopBefore(r, 2), "p1", nil, nil), turnView(r, lastSeqOf(r), "p2", nil, nil)
	t1, _ := turnOf(one.Obligations)
	t2, _ := turnOf(two.Obligations)
	if len(t1) != 1 || len(t2) != 1 || t1[0].Kind != GateAfterWrite || t2[0].Kind != GateAfterWrite {
		t.Fatalf("turn rows %+v and %+v, want one gate_after_write row each", t1, t2)
	}
	if t1[0].ID == t2[0].ID {
		t.Errorf("two turns share the id %s", t1[0].ID)
	}
	if s1, s2 := one.Obligations[0], two.Obligations[0]; s1.Kind != GateAfterWrite || s1.ID != s2.ID {
		t.Errorf("witness: session rows %+v and %+v should share their id", s1, s2)
	}
}

// TestLedger_TwoStopsInOneTurnShareIDs: two Stops in one turn give its rows
// the same ids. Witness: the second Stop read them anew (unmet, then met).
func TestLedger_TwoStopsInOneTurnShareIDs(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed), edit(3, "pkg/a.go"),
		gateCall(4, "go-test", store.ExecOK)), "p1", "p1", "p1", "p1")
	early, _ := turnOf(turnView(r, stopBefore(r, 2), "p1", nil, nil).Obligations)
	late, _ := turnOf(turnView(r, lastSeqOf(r), "p1", nil, nil).Obligations)
	want := []ObligationKind{GateAfterWrite, WatchedRedThenGreen}
	for _, rows := range [][]Obligation{early, late} {
		var kinds []ObligationKind
		for _, o := range rows {
			kinds = append(kinds, o.Kind)
		}
		if !slices.Equal(kinds, want) {
			t.Fatalf("turn rows %+v, want kinds %v", rows, want)
		}
	}
	for i := range early {
		if early[i].ID != late[i].ID {
			t.Errorf("%s: ids %s and %s in one turn", early[i].Kind, early[i].ID, late[i].ID)
		}
	}
	if early[0].State != ObligationUnmet || late[0].State != ObligationMet {
		t.Errorf("witness: gate_after_write %s then %s, want unmet then met", early[0].State, late[0].State)
	}
}

// TestLedger_UnitRowPerUnitWritten: one turn row per unit the turn wrote,
// its Unit UnitOf's id, in unit id order, its id TurnObligationID's over the
// unit.
func TestLedger_UnitRowPerUnitWritten(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), edit(2, "pkg/a/y.go"), edit(3, "pkg/b/z.go"),
		gateCall(4, "go-test", store.ExecOK)), "p1", "p1", "p1", "p1")
	at := lastSeqOf(r)
	v := turnView(r, at, "p1", dirUnit, nil)
	g, w := one(t, v.Snap.Gates), TurnWindow(r, "p1", at)
	_, units := turnOf(v.Obligations)
	var got []string
	for _, o := range units {
		got = append(got, o.Unit)
		if o.Kind != GateAfterWrite || o.State != ObligationMet || o.ID != TurnObligationID(GateAfterWrite, idFamily(&g), w, o.Unit) {
			t.Errorf("unit row %+v", o)
		}
	}
	if want := []string{"g:r#pkg/a", "g:r#pkg/b"}; !slices.Equal(got, want) {
		t.Errorf("units %q, want %q", got, want)
	}
}

// TestLedger_UnitRowReadsItsOwnWrites: a unit row is its gate's table with
// that unit's writes alone counted. A gate stale by a write to one unit after
// its green is met for a unit written before the green, citing the green,
// and unmet for the unit written since, citing the run and that write.
// Witness: the gate's own tree bind reads foreign, the move of the second
// unit's path under a named write, which would leave the first unit's row
// unknown were it read for that unit.
func TestLedger_UnitRowReadsItsOwnWrites(t *testing.T) {
	r := inTurns(runOf(t, hashedEdit(1, "pkg/a/x.go", shaA), hashedEdit(2, "pkg/b/y.go", shaA),
		gateCall(3, "go-test", store.ExecOK), hashedEdit(4, "pkg/b/y.go", shaB)), "p1", "p1", "p1", "p1")
	v := turnView(r, lastSeqOf(r), "p1", dirUnit, map[string]*string{"pkg/a/x.go": &shaA, "pkg/b/y.go": &shaB})
	if g := one(t, v.Snap.Gates); g.State != Stale || g.Reason != WritesAfterGreen || g.bound != boundForeign {
		t.Fatalf("gate %s %s bound %d, want stale, writes_after_green, bound foreign", g.State, g.Reason, g.bound)
	}
	green, since := r.Declarations[2].Seq, r.Declarations[3].Seq
	want := []Obligation{
		{Unit: "g:r#pkg/a", State: ObligationMet, Evidence: []int64{green}},
		{Unit: "g:r#pkg/b", State: ObligationUnmet, Evidence: []int64{green, since}},
	}
	_, units := turnOf(v.Obligations)
	if len(units) != len(want) {
		t.Fatalf("unit rows %+v, want %d", units, len(want))
	}
	for i, o := range units {
		if o.Unit != want[i].Unit || o.State != want[i].State || !slices.Equal(o.Evidence, want[i].Evidence) {
			t.Errorf("unit row %s %s %v, want %s %s %v", o.Unit, o.State, o.Evidence, want[i].Unit, want[i].State, want[i].Evidence)
		}
	}
}

// TestLedger_TurnWindowOwesOnlyItsWrites: a write before the turn's From owes
// no turn row: the second turn's unit rows are its own unit's, and a turn
// that wrote nothing owes no gate_after_write row. Witness: the session row
// is still owed.
func TestLedger_TurnWindowOwesOnlyItsWrites(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/b/z.go"),
		gateCall(4, "go-test", store.ExecOK), gateCall(5, "go-test", store.ExecOK)), "p1", "p1", "p2", "p2", "p3")
	if _, units := turnOf(turnView(r, stopBefore(r, 4), "p2", dirUnit, nil).Obligations); len(units) != 1 || units[0].Unit != "g:r#pkg/b" {
		t.Errorf("the second turn's unit rows %+v, want pkg/b's alone", units)
	}
	v := turnView(r, lastSeqOf(r), "p3", dirUnit, nil)
	if turn, _ := turnOf(v.Obligations); len(turn) != 0 {
		t.Errorf("a turn that wrote nothing owes %+v", turn)
	}
	if s := v.Obligations[0]; s.Scope != ScopeSession || s.Kind != GateAfterWrite {
		t.Errorf("witness: first row %+v, want the session's gate_after_write", s)
	}
}

// TestLedger_UnitRowCapBindsAndCounts: with MaxTurnUnitRows units written the
// Stop keeps every unit row and UnitRowsCut is 0; with one unit more it keeps
// MaxTurnUnitRows, in gate order then unit id ascending, and UnitRowsCut is 1;
// with two gates over 17 units the second gate's last two are cut.
func TestLedger_UnitRowCapBindsAndCounts(t *testing.T) {
	for _, c := range []struct {
		units   int
		runners []string
		cut     int
	}{{MaxTurnUnitRows, []string{"go-test"}, 0}, {MaxTurnUnitRows + 1, []string{"go-test"}, 1}, {17, []string{"go-test", "go-vet"}, 2}} {
		var calls []call
		for i := range c.units {
			calls = append(calls, edit(int64(i+1), fmt.Sprintf("u%02d/f.go", i)))
		}
		for j, rn := range c.runners {
			calls = append(calls, gateCall(int64(c.units+j+1), rn, store.ExecOK))
		}
		ids := slices.Repeat([]string{"p1"}, len(calls))
		r := inTurns(runOf(t, calls...), ids...)
		at := lastSeqOf(r)
		v, w := turnView(r, at, "p1", dirUnit, nil), TurnWindow(r, "p1", at)
		var want []string
		for i := range v.Snap.Gates {
			for u := range c.units {
				want = append(want, TurnObligationID(GateAfterWrite, idFamily(&v.Snap.Gates[i]), w, fmt.Sprintf("g:r#u%02d", u)))
			}
		}
		var got []string
		_, units := turnOf(v.Obligations)
		for _, o := range units {
			got = append(got, o.ID)
		}
		if v.UnitRowsCut != c.cut || len(want) != MaxTurnUnitRows+c.cut || !slices.Equal(got, want[:MaxTurnUnitRows]) {
			t.Errorf("%d units, %d gates: kept %d, cut %d; want %d kept in order, cut %d", c.units, len(c.runners), len(got), v.UnitRowsCut, MaxTurnUnitRows, c.cut)
		}
	}
}

// TestLedger_SubtaskPointAddsNoTurnRows: a subtask or commit point's ledger
// is its session rows only, as is every point's but a Stop's. Witness: the
// Stop's has turn rows.
func TestLedger_SubtaskPointAddsNoTurnRows(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK)), "p1", "p1")
	for _, k := range PointKinds {
		v := EvaluateStop(r, nil, nil, Point{Kind: k, Seq: lastSeqOf(r), SessionID: "s1"}, stopNow, StopInput{PromptID: "p1", UnitOf: dirUnit})
		if turn, _ := turnOf(v.Obligations); (len(turn) > 0) != (k == PointStop) || v.UnitRowsCut != 0 {
			t.Errorf("point %s: turn rows %+v, cut %d; want rows at a Stop alone", k, turn, v.UnitRowsCut)
		}
	}
}

// TestLedger_NilUnitOfAddsNoUnitRow: with UnitOf nil no turn row carries a
// Unit and the gate's turn row stays; with it set, the unit rows come
// (witness).
func TestLedger_NilUnitOfAddsNoUnitRow(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), edit(2, "pkg/b/z.go"), gateCall(3, "go-test", store.ExecOK)), "p1", "p1", "p1")
	at := lastSeqOf(r)
	if turn, units := turnOf(turnView(r, at, "p1", nil, nil).Obligations); len(turn) != 1 || len(units) != 0 {
		t.Errorf("UnitOf nil: turn rows %+v, want the gate's one and no unit row", turn)
	}
	if _, units := turnOf(turnView(r, at, "p1", dirUnit, nil).Obligations); len(units) != 2 {
		t.Errorf("witness: UnitOf set gives unit rows %+v, want 2", units)
	}
}

// TestLedger_Replay: each Stop's ledger, turn rows included, is the ledger
// over the record's prefix at its seq (INV-11), read once in the prompt id
// the Stop records, as the ledger verb's replay passes its checkpoint's, and
// once with no prompt id, which resolves to the turn current at the seq
// (card W-M3.1, Q4). Witness: in each pass, Stops in each of the three turns
// hold turn rows; mutant TL-1 (the bound dropped) fails the unnamed pass at
// a seq inside p1, where the whole record resolves to p3.
func TestLedger_Replay(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecFailed), edit(3, "pkg/b/y.go"),
		gateCall(4, "go-test", store.ExecOK), edit(5, "pkg/a/x.go"), gateCall(6, "go-vet", store.ExecOK)),
		"p1", "p1", "p2", "p2", "p3", "p3")
	for _, named := range []bool{true, false} {
		turned := map[string]bool{}
		for s := int64(0); s <= lastSeqOf(r); s++ {
			p, id := promptAt(r, s), ""
			if named {
				id = p
			}
			a, b := turnView(r, s, id, dirUnit, nil), turnView(prefixOf(r, s), s, id, dirUnit, nil)
			if ja, jb := encodeObs(t, a.Obligations), encodeObs(t, b.Obligations); !bytes.Equal(ja, jb) || a.UnitRowsCut != b.UnitRowsCut {
				t.Fatalf("named %v: at %d the ledger differs from the prefix's:\n%s\n%s", named, s, ja, jb)
			}
			if turn, _ := turnOf(a.Obligations); len(turn) > 0 {
				turned[p] = true
			}
		}
		if len(turned) != 3 {
			t.Errorf("witness: named %v: turns with turn rows %v, want p1, p2 and p3", named, turned)
		}
	}
}

// TestCountTurn_EmptyIDReadsTheTurnAtThePoint: countTurn with no prompt id
// counts the turn current at its seq, so over the whole record it counts
// what it counts over the record's prefix at that seq (card W-M3.1, Q4;
// INV-11). Witness: a seq inside p1 counts p1's write; mutant TL-1 counts
// nothing there over the whole record, which resolves to p2.
func TestCountTurn_EmptyIDReadsTheTurnAtThePoint(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/b/y.go"),
		edit(4, "pkg/c/z.go")), "p1", "p1", "p2", "p2")
	c, first := classifiedOf(r), r.Declarations[0].Seq
	inP1 := false
	for s := int64(0); s <= lastSeqOf(r); s++ {
		pre := prefixOf(r, s)
		if got, want := countTurn(r, c, "", s, nil), countTurn(pre, classifiedOf(pre), "", s, nil); got != want {
			t.Errorf("at %d: countTurn over the record %+v, over its prefix %+v", s, got, want)
		} else if got.Writes > 0 && got.FirstSeq == first {
			inP1 = true
		}
	}
	if !inP1 {
		t.Error("witness: no seq inside p1 counts p1's write")
	}
}

// TestLedger_EmptyWindowAddsNoTurnRow: a Stop whose turn has no call by its
// seq, the empty window, adds no turn row: its From, past its To, may be a
// later turn's first call, whose rows would then share its ids. So does a
// run where no call carries a prompt id. Witness: the turn that made the
// calls has rows.
func TestLedger_EmptyWindowAddsNoTurnRow(t *testing.T) {
	calls := []call{edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK)}
	r := inTurns(runOf(t, calls...), "p1", "p1")
	at := lastSeqOf(r)
	for _, c := range []struct {
		name   string
		r      *store.Run
		prompt string
	}{{"a prompt with no call by the Stop", r, "p2"}, {"no call carries a prompt id", runOf(t, calls...), ""}} {
		if !TurnWindow(c.r, c.prompt, at).Empty() {
			t.Fatalf("%s: the window is not empty", c.name)
		}
		if turn, _ := turnOf(turnView(c.r, at, c.prompt, dirUnit, nil).Obligations); len(turn) != 0 {
			t.Errorf("%s: turn rows %+v", c.name, turn)
		}
	}
	if turn, _ := turnOf(turnView(r, at, "p1", dirUnit, nil).Obligations); len(turn) == 0 {
		t.Error("witness: the turn that made the calls has no turn rows")
	}
}

// TestLedger_RedInAnEarlierTurnOwesNoWatchedRow: a family's last red owes a
// turn its watched_red_then_green row only where the red is a call of that
// turn, its seq at or after From (ledger_turn.go): a red in p1 and its green
// in p2, so p2's Stop owes its gate_after_write row alone. Witness: the
// session owes the watched row.
func TestLedger_RedInAnEarlierTurnOwesNoWatchedRow(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed), edit(3, "pkg/a.go"),
		gateCall(4, "go-test", store.ExecOK)), "p1", "p1", "p2", "p2")
	v := turnView(r, lastSeqOf(r), "p2", nil, nil)
	if turn, _ := turnOf(v.Obligations); len(turn) != 1 || turn[0].Kind != GateAfterWrite {
		t.Errorf("p2's turn rows %+v, want its gate_after_write row alone", turn)
	}
	if !slices.ContainsFunc(v.Obligations, func(o Obligation) bool {
		return o.Scope == ScopeSession && o.Kind == WatchedRedThenGreen
	}) {
		t.Errorf("witness: no session watched_red_then_green row in %+v", v.Obligations)
	}
}

// TestLedger_TurnRowsReadUnknownWhereTheSessionRowsDo: a gate's turn rows,
// its gate_after_write row and its unit row, read unknown where its session
// rows do (ledger_turn.go, obligations): with the recording incomplete, with
// a source outside the four names, and with a state outside Order. Each case
// reads a fresh view's snapshot, the one EvaluateStop read, and keeps the
// row count. Witness: the snapshot as read gives both turn rows, met. The
// state case is the table's as well (fromTable reads unknown for a state it
// lacks); the recording and source cases are the guard's alone. The fourth
// case is a turn that went red, then green after a write: its
// watched_red_then_green row takes its state from the watch (watchedRow),
// never from a table, so for a state outside Order the guard alone reads it
// unknown. Witness: as read, that row is met.
func TestLedger_TurnRowsReadUnknownWhereTheSessionRowsDo(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK)), "p1", "p1")
	at, c := lastSeqOf(r), classifiedOf(r)
	rows := func(snap Snapshot) []Obligation {
		turn, _ := turnObligations(snap, &c, r, "p1", "", dirUnit)
		return turn
	}
	base := rows(turnView(r, at, "p1", dirUnit, nil).Snap)
	if len(base) != 2 || base[0].State != ObligationMet || base[1].State != ObligationMet || base[1].Unit == "" {
		t.Fatalf("witness: turn rows %+v, want the gate's and its unit's, met", base)
	}
	for _, k := range []struct {
		name string
		set  func(*Snapshot)
	}{
		{"recording incomplete", func(s *Snapshot) { s.Recording = Incomplete }},
		{"source outside the four", func(s *Snapshot) { s.Gates[0].Source = "elsewhere" }},
		{"state outside Order", func(s *Snapshot) { s.Gates[0].State = "unheard" }},
	} {
		snap := turnView(r, at, "p1", dirUnit, nil).Snap
		k.set(&snap)
		got, session := rows(snap), obligations(snap, c.runs, c.writes)
		if len(got) != len(base) || len(session) == 0 {
			t.Errorf("%s: %d turn rows and %d session rows, want %d and some", k.name, len(got), len(session), len(base))
		}
		for _, o := range append(got, session...) {
			if o.State != ObligationUnknown {
				t.Errorf("%s: %s row %s reads %s, want unknown", k.name, o.Scope, o.Kind, o.State)
			}
		}
	}
	rw := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecFailed),
		edit(3, "pkg/a/x.go"), gateCall(4, "go-test", store.ExecOK)), "p1", "p1", "p1", "p1")
	atw, cw := lastSeqOf(rw), classifiedOf(rw)
	watched := func(state ObligationState) func(Obligation) bool {
		return func(o Obligation) bool { return o.Kind == WatchedRedThenGreen && (state == "" || o.State == state) }
	}
	snap := turnView(rw, atw, "p1", dirUnit, nil).Snap
	if before, _ := turnObligations(snap, &cw, rw, "p1", "", dirUnit); !slices.ContainsFunc(before, watched(ObligationMet)) {
		t.Fatalf("witness: turn rows %+v, want a watched_red_then_green row, met", before)
	}
	snap.Gates[0].State = "unheard"
	got, _ := turnObligations(snap, &cw, rw, "p1", "", dirUnit)
	session := obligations(snap, cw.runs, cw.writes)
	if !slices.ContainsFunc(got, watched("")) || len(session) == 0 {
		t.Errorf("state outside Order, a watched turn: turn rows %+v and %d session rows, want the watched row and some", got, len(session))
	}
	for _, o := range append(got, session...) {
		if o.State != ObligationUnknown {
			t.Errorf("state outside Order, a watched turn: %s row %s reads %s, want unknown", o.Scope, o.Kind, o.State)
		}
	}
}

// TestTurnObligations_ATurnWithNoWriteCallsNoUnitOf (B2): UnitOf is asked
// about a written path only, so a Stop whose turn wrote nothing asks it
// nothing, and the Stop's UnitOf, which opens its files when first asked,
// opens none. Witness: a turn with a write asks it.
func TestTurnObligations_ATurnWithNoWriteCallsNoUnitOf(t *testing.T) {
	for _, c := range []struct {
		name  string
		run   *store.Run
		calls bool
	}{
		{"no write", inTurns(runOf(t, gateCall(1, "go-test", store.ExecOK)), "p1"), false},
		{"a write", inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK)), "p1", "p1"), true},
	} {
		n := 0
		count := func(rel string) string { n++; return dirUnit(rel) }
		pt := Point{Kind: PointStop, Seq: lastSeqOf(c.run), SessionID: "s1"}
		EvaluateStop(c.run, nil, nil, pt, stopNow, StopInput{PromptID: "p1", UnitOf: count})
		if (n > 0) != c.calls {
			t.Errorf("%s: UnitOf asked %d times; want asked %v", c.name, n, c.calls)
		}
	}
}

// TestPlaced_PutsAWriteUnderTheStopsRoot (W-M3.3): UnitOf reads a written
// path relative to the Stop's project, so a path left relative to its own
// call's project is placed by where that project sits against the root, an
// absolute one is cut to the root, and one with no place under the root is
// not placed.
func TestPlaced_PutsAWriteUnderTheStopsRoot(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, c := range []struct {
		name, root string
		project    *string
		path, want string
		ok         bool
	}{
		{"no root: the path as recorded", "", str("/q"), "a/x.go", "a/x.go", true},
		{"the project is the root", "/r", str("/r"), "a/x.go", "a/x.go", true},
		{"the project inside the root", "/r", str("/r/w"), "a/x.go", "w/a/x.go", true},
		{"the root inside the project", "/r/wt", str("/r"), "wt/a/x.go", "a/x.go", true},
		{"the root inside the project, the path elsewhere", "/r/wt", str("/r"), "o/x.go", "", false},
		{"absolute under the root", "/r", str("/q"), "/r/a/x.go", "a/x.go", true},
		{"absolute outside the root", "/r", str("/r"), "/q/a/x.go", "", false},
		{"climbs out", "/r", str("/r"), "../x.go", "", false},
		{"the project not said", "/r", nil, "a/x.go", "", false},
		{"the project apart from the root", "/r", str("/q"), "a/x.go", "", false},
	} {
		w := write{Path: &c.path}
		if c.project != nil {
			w.decl = &store.Declaration{}
			w.decl.Project = c.project
		}
		if got, ok := placed(c.root, &w); ok != c.ok || ok && got != c.want {
			t.Errorf("%s: placed = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestLedger_UnitRowReadsThePathUnderTheStopsRoot (W-M3.3): a turn's unit
// row reads the written path placed under StopInput.Root, so a session whose
// project is a directory inside the root names the unit by its path from
// the root.
func TestLedger_UnitRowReadsThePathUnderTheStopsRoot(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecOK)), "p1", "p1")
	proj := "/r/w"
	for i := range r.Declarations {
		r.Declarations[i].Project = &proj
	}
	pt := Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1"}
	v := EvaluateStop(r, nil, nil, pt, stopNow, StopInput{PromptID: "p1", UnitOf: dirUnit, Root: "/r"})
	_, units := turnOf(v.Obligations)
	if len(units) != 1 || units[0].Unit != "g:r#w/pkg/a" {
		t.Errorf("unit rows %+v, want one, its unit g:r#w/pkg/a", units)
	}
}
