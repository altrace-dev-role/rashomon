package verify

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestScopes_ClosedList: Scopes is session, subtask, commit and turn, by
// their wire values, and every scope a point kind owes in is one of them
// (W-M3.1). No scope is a unit's.
func TestScopes_ClosedList(t *testing.T) {
	if want := []string{"session", "subtask", "commit", "turn"}; !slices.Equal(Scopes, want) {
		t.Errorf("Scopes %q, want %q", Scopes, want)
	}
	for kind, scope := range obligationScopes() {
		if !slices.Contains(Scopes, scope) {
			t.Errorf("point %s owes in %q, not in Scopes", kind, scope)
		}
	}
}

// TestPreds_ClosedList: Preds is the four requirement predicates by their
// wire values (W-M3.4).
func TestPreds_ClosedList(t *testing.T) {
	if want := []string{"file_exists", "symbol_exists", "gate_green", "test_named"}; !slices.Equal(Preds, want) {
		t.Errorf("Preds %q, want %q", Preds, want)
	}
}

// promptRun is a run of one declaration per entry of ids, at seqs 1, 2, ...,
// each carrying that prompt id; "" declares a call with none.
func promptRun(ids ...string) *store.Run {
	r := &store.Run{}
	for i := range ids {
		d := store.Declaration{Seq: int64(i + 1)}
		if ids[i] != "" {
			d.PromptID = &ids[i]
		}
		r.Declarations = append(r.Declarations, d)
	}
	return r
}

// TestTurnWindow_StartsAtTheTurnsFirstCall: a turn's window runs from its
// prompt's first call to the point, and its From is countTurn's FirstSeq,
// one notion of a turn; a prompt with no call by the point has the empty
// window, where countTurn counts none.
func TestTurnWindow_StartsAtTheTurnsFirstCall(t *testing.T) {
	r := promptRun("p1", "p1", "p2", "p2", "p2")
	for _, c := range []struct {
		prompt    string
		seq, from int64
	}{{"p2", 5, 3}, {"p2", 4, 3}, {"p1", 2, 1}, {"p2", 2, 3}, {"p3", 5, 6}} {
		w := TurnWindow(r, c.prompt, c.seq)
		if w != (Window{Scope: ScopeTurn, From: c.from, To: c.seq}) {
			t.Errorf("TurnWindow(%s, %d) = %+v, want From %d To %d", c.prompt, c.seq, w, c.from, c.seq)
		}
		first := countTurn(r, classified{}, c.prompt, c.seq, nil).FirstSeq
		if (w.Empty() && first != 0) || (!w.Empty() && first != w.From) {
			t.Errorf("TurnWindow(%s, %d) = %+v, countTurn's FirstSeq %d", c.prompt, c.seq, w, first)
		}
	}
	if !TurnWindow(r, "p2", 2).Empty() || TurnWindow(r, "p2", 3).Empty() {
		t.Error("p2's window empty before its first call, and only before it")
	}
}

// TestTurnWindow_EmptyPromptIDResolvesAsCountTurn: an empty prompt id is the
// latest prompt's turn, as countTurn reads it; only where no declaration
// carries a prompt id does the window start at the run's first call, and an
// empty run's window is empty.
func TestTurnWindow_EmptyPromptIDResolvesAsCountTurn(t *testing.T) {
	r := promptRun("p1", "p1", "p2", "p2")
	if got, want := TurnWindow(r, "", 4), TurnWindow(r, "p2", 4); got != want || got.From != 3 {
		t.Errorf("TurnWindow(\"\", 4) = %+v, want p2's %+v", got, want)
	}
	if got := TurnWindow(promptRun("", "", ""), "", 3); got != (Window{Scope: ScopeTurn, From: 1, To: 3}) {
		t.Errorf("no prompt ids: %+v, want From 1 To 3", got)
	}
	if got := TurnWindow(&store.Run{}, "", 7); !got.Empty() {
		t.Errorf("empty run: %+v, want the empty window", got)
	}
}

// TestTurnWindow_EveryPromptIDStartsATurn: each distinct prompt id starts a
// turn, whatever sent it; the record names no sender, so a person's, a peer's
// and a scheduled prompt each have a window of their own calls.
func TestTurnWindow_EveryPromptIDStartsATurn(t *testing.T) {
	r := promptRun("person", "peer", "peer", "cron")
	for _, c := range []struct {
		prompt    string
		seq, from int64
	}{{"person", 1, 1}, {"peer", 3, 2}, {"cron", 4, 4}} {
		if w := TurnWindow(r, c.prompt, c.seq); w.From != c.from || w.To != c.seq {
			t.Errorf("%s by %d: %+v, want From %d", c.prompt, c.seq, w, c.from)
		}
	}
}

// TestTurnObligationID_CarriesTheWindowsFrom: two Stops in one turn give a
// row one id, To not in the key; two turns give two; a unit gives its own; a
// turn row never takes the session row's id. Witness: the key without its
// family length is ambiguous, "a1" and "" against "a" and "1".
func TestTurnObligationID_CarriesTheWindowsFrom(t *testing.T) {
	early := TurnObligationID(GateAfterWrite, "go-test", Window{ScopeTurn, 5, 9}, "")
	if late := TurnObligationID(GateAfterWrite, "go-test", Window{ScopeTurn, 5, 12}, ""); late != early {
		t.Errorf("one turn, two Stops: %s and %s", early, late)
	}
	if next := TurnObligationID(GateAfterWrite, "go-test", Window{ScopeTurn, 13, 20}, ""); next == early {
		t.Errorf("two turns share id %s", next)
	}
	w := Window{ScopeTurn, 5, 9}
	if TurnObligationID(GateAfterWrite, "go-test", w, "g:r#a") == TurnObligationID(GateAfterWrite, "go-test", w, "g:r#b") {
		t.Error("two units share a turn row's id")
	}
	if early == ObligationID(ScopeSession, GateAfterWrite, "go-test") {
		t.Error("a turn row takes the session row's id")
	}
	if TurnObligationID(GateAfterWrite, "a1", w, "") == TurnObligationID(GateAfterWrite, "a", w, "1") {
		t.Error(`family "a1" unit "" and family "a" unit "1" share an id`)
	}
	if ObligationID(ScopeTurn, GateAfterWrite, "t5:a1"+"") != ObligationID(ScopeTurn, GateAfterWrite, "t5:a"+"1") {
		t.Error("witness: the key without the family's length should be ambiguous")
	}
}

// TestReqFamily_NeverATaskRowsID: a requirement's family is "req:" and its
// id, keyed into ObligationID, so its row has an ob: id, and no task evidence
// kind starts "req:": a requirement named as a task kind keeps an id of its
// own.
func TestReqFamily_NeverATaskRowsID(t *testing.T) {
	if got := ReqFamily("r1"); got != "req:r1" {
		t.Errorf("ReqFamily(r1) = %q", got)
	}
	if id := ObligationID(ScopeSession, ExpectedEvidence, ReqFamily("r1")); !strings.HasPrefix(id, "ob:") || len(id) != 19 {
		t.Errorf("row id %q, want ob: and 16 hex", id)
	}
	for _, k := range spec.EvidenceKinds {
		if strings.HasPrefix(k, "req:") {
			t.Errorf("task evidence kind %q starts req:", k)
		}
		if ObligationID(ScopeSession, ExpectedEvidence, ReqFamily(k)) == ObligationID(ScopeSession, ExpectedEvidence, k) {
			t.Errorf("requirement %q takes the task row's id", k)
		}
	}
}

// TestPoint_ContentIsNotEncoded: a point encodes the same with Content set
// as without it, so a checkpoint's bytes do not move (W-M3.2). Witness: a
// func field the encoder is not told to skip fails to encode.
func TestPoint_ContentIsNotEncoded(t *testing.T) {
	p := Point{Kind: PointStop, Seq: 3, SessionID: "s1"}
	want, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Content = func(string) ([]byte, bool) { return []byte("x"), true }
	if got, err := json.Marshal(p); err != nil || string(got) != string(want) {
		t.Errorf("with Content: %s, %v; want %s", got, err, want)
	}
	if _, err := json.Marshal(struct{ F func() }{F: func() {}}); err == nil {
		t.Error("witness: an untagged func field should fail to encode")
	}
}

// sessionRowsGolden is the session rows of TestLedger_SessionRowsUnchanged's
// fixture before turn rows (W-M3.1).
const sessionRowsGolden = `[{"id":"ob:c98bf08ab0bd81d7","scope":"session","kind":"gate_after_write","source":"learned","state":"met","evidence":[7]},{"id":"ob:11e31c0d7bf7bbd2","scope":"session","kind":"watched_red_then_green","source":"learned","state":"met","evidence":[3,5,7]},{"id":"ob:6b7baf7f0ce64459","scope":"session","kind":"exit_unmasked","source":"learned","state":"met","evidence":[7]},{"id":"ob:36c9f6491c502aec","scope":"session","kind":"evidence_bound","source":"learned","state":"unknown","evidence":[]},{"id":"ob:3e1d3a87547fafb3","scope":"session","kind":"expected_evidence","source":"task","state":"unmet","evidence":[]}]`

// TestLedger_SessionRowsUnchanged: the session rows lead the ledger, unbroken,
// and encode to the golden at a Stop, a subtask and a commit point, with
// UnitOf nil and set (W-M3.1): turn rows, when they come, change none of
// them.
func TestLedger_SessionRowsUnchanged(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed), edit(3, "pkg/a.go"), gateCall(4, "go-test", store.ExecOK))
	sp := evSpec([]string{"cross_check"}, "")
	for _, kind := range []string{PointStop, PointSubtask, PointCommit} {
		for i, u := range []UnitOf{nil, func(rel string) string { return "g:r#" + rel }} {
			pt := Point{Kind: kind, Seq: lastSeqOf(r), SessionID: "s1"}
			v := EvaluateStop(r, nil, sp, pt, stopNow, StopInput{UnitOf: u})
			if got := sessionPrefix(t, v.Obligations); got != sessionRowsGolden {
				t.Errorf("%s, UnitOf %d: session rows\n got %s\nwant %s", kind, i, got, sessionRowsGolden)
			}
		}
	}
}

// sessionPrefix is the JSON of the session rows that lead obs; it fails the
// test on a session row after any other.
func sessionPrefix(t *testing.T, obs []Obligation) string {
	t.Helper()
	n := 0
	for n < len(obs) && obs[n].Scope == ScopeSession {
		n++
	}
	for _, o := range obs[n:] {
		if o.Scope == ScopeSession {
			t.Errorf("session row %s after a %s row", o.ID, obs[n].Scope)
		}
	}
	b, err := json.Marshal(obs[:n])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
