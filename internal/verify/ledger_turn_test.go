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
// over the record's prefix at its seq (INV-11), each read in the prompt id
// the Stop records, as the ledger verb's replay passes its checkpoint's.
// Witness: Stops in each of the three turns hold turn rows.
func TestLedger_Replay(t *testing.T) {
	r := inTurns(runOf(t, edit(1, "pkg/a/x.go"), gateCall(2, "go-test", store.ExecFailed), edit(3, "pkg/b/y.go"),
		gateCall(4, "go-test", store.ExecOK), edit(5, "pkg/a/x.go"), gateCall(6, "go-vet", store.ExecOK)),
		"p1", "p1", "p2", "p2", "p3", "p3")
	turned := map[string]bool{}
	for s := int64(0); s <= lastSeqOf(r); s++ {
		p := promptAt(r, s)
		a, b := turnView(r, s, p, dirUnit, nil), turnView(prefixOf(r, s), s, p, dirUnit, nil)
		if ja, jb := encodeObs(t, a.Obligations), encodeObs(t, b.Obligations); !bytes.Equal(ja, jb) || a.UnitRowsCut != b.UnitRowsCut {
			t.Fatalf("at %d the ledger differs from the prefix's:\n%s\n%s", s, ja, jb)
		}
		if turn, _ := turnOf(a.Obligations); len(turn) > 0 {
			turned[p] = true
		}
	}
	if len(turned) != 3 {
		t.Errorf("witness: turns with turn rows %v, want p1, p2 and p3", turned)
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
