package verify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// rowByID is the one row of obs whose ID is id; it fails the test unless there
// is exactly one.
func rowByID(t *testing.T, obs []Obligation, id string) Obligation {
	t.Helper()
	var got []Obligation
	for _, o := range obs {
		if o.ID == id {
			got = append(got, o)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%d rows with id %s, want one: %+v", len(got), id, obs)
	}
	return got[0]
}

// TestLW1_StopViewCarriesTheLedger: the Stop view carries the ledger over its
// own classification (card LEDGER-WIRE1). A green run after a write owes a
// gate_after_write row met on the green run, every row in the session scope;
// an empty run's ledger is empty and never null.
func TestLW1_StopViewCarriesTheLedger(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	v := stopOf(r, nil, "s1", StopInput{})
	wantRow(t, rowOf(t, v.Obligations, GateAfterWrite), ObligationMet, declSeq(r, 1))
	for _, o := range v.Obligations {
		if o.Scope != ScopeSession {
			t.Errorf("%s: scope %q, want session", o.Kind, o.Scope)
		}
	}
	if v := stopOf(&store.Run{}, nil, "s1", StopInput{}); v.Obligations == nil || len(v.Obligations) != 0 {
		t.Errorf("an empty run's ledger %#v, want an empty, non-nil list", v.Obligations)
	}
}

// TestLW1b_LedgerRowsReadTheAtStopExpected: the ledger's expected_evidence rows
// are the Stop view's Expected rows, read at Stop: a run kind no gate supplies
// is unmet at Stop (open before it), and its row says unmet in the session
// scope under the kind's id, citing nothing. Each Expected row has its ledger
// row in the same state, so the ledger and the done flag's clause agree.
func TestLW1b_LedgerRowsReadTheAtStopExpected(t *testing.T) {
	sp := evSpec([]string{"cross_check", "judgment_disclosed"}, "")
	r := runOf(t, edit(1, "pkg/a.go"))
	v := EvaluateStop(r, nil, sp, Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1"}, stopNow, StopInput{})
	if len(v.Expected) != 2 {
		t.Fatalf("Expected %+v, want the task's two kinds", v.Expected)
	}
	cross := rowByID(t, v.Obligations, ObligationID(ScopeSession, ExpectedEvidence, "cross_check"))
	if cross.Kind != ExpectedEvidence || cross.Scope != ScopeSession || cross.Source != SourceTask {
		t.Errorf("cross_check row %+v, want expected_evidence, session, task", cross)
	}
	wantRow(t, cross, ObligationUnmet)
	for _, e := range v.Expected {
		if o := rowByID(t, v.Obligations, ObligationID(ScopeSession, ExpectedEvidence, e.Kind)); o.State != e.State {
			t.Errorf("%s: the ledger says %s, the Stop's Expected %s", e.Kind, o.State, e.State)
		}
	}
}

// TestLW1f_UnreadMessageReadsJudgmentUnknown: judgment_disclosed is read from
// the final message. With the message read and nothing disclosed it is unmet
// at Stop; with no message held (a replay from the record, which keeps none)
// it is unknown, never unmet (INV-2), in the Expected rows and the ledger
// alike, and UnmetExpected does not name it.
func TestLW1f_UnreadMessageReadsJudgmentUnknown(t *testing.T) {
	sp := evSpec([]string{"judgment_disclosed"}, "")
	r := runOf(t, edit(1, "pkg/a.go"))
	pt := Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1"}
	id := ObligationID(ScopeSession, ExpectedEvidence, "judgment_disclosed")
	for _, c := range []struct {
		name string
		in   StopInput
		want ObligationState
	}{
		{"the message read, nothing disclosed", StopInput{}, ObligationUnmet},
		{"no message held", StopInput{MessageUnread: true}, ObligationUnknown},
	} {
		v := EvaluateStop(r, nil, sp, pt, stopNow, c.in)
		if got := oneRow(t, v.Expected); got.State != c.want {
			t.Errorf("%s: Expected %+v, want %s", c.name, got, c.want)
		}
		if o := rowByID(t, v.Obligations, id); o.State != c.want {
			t.Errorf("%s: ledger row %+v, want %s", c.name, o, c.want)
		}
		if c.want == ObligationUnknown && len(UnmetExpected(v.Expected, v.Obligations)) != 0 {
			t.Errorf("%s: UnmetExpected %+v, want none", c.name, UnmetExpected(v.Expected, v.Obligations))
		}
	}
}

// TestLW1g_OneLedgerProducer: the ledger has one producer, on the one path
// every Stop takes: obligations and expectedObligations are called from
// ledgerOf alone, and ledgerOf from EvaluateStop alone, so no second path can
// build rows the Stop's would disagree with. Read from the package's source,
// test files aside.
func TestLW1g_OneLedgerProducer(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	callers := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && slices.Contains([]string{"obligations", "expectedObligations", "ledgerOf"}, id.Name) {
						callers[id.Name] = append(callers[id.Name], fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	want := map[string][]string{"obligations": {"ledgerOf"}, "expectedObligations": {"ledgerOf"}, "ledgerOf": {"EvaluateStop"}}
	if !reflect.DeepEqual(callers, want) {
		t.Errorf("callers %v, want %v", callers, want)
	}
}

// TestLW1l_PinnedRequiredDecidesTheRows: a replay pins each gate its
// checkpoint names to the Required recorded there. With no index every gate
// is required; pinned advisory, a red gate owes no row and no longer holds
// the record green back; a gate the pins do not name keeps evaluate's.
func TestLW1l_PinnedRequiredDecidesTheRows(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), gateCall(3, "go-vet", store.ExecFailed))
	free := stopOf(r, nil, "s1", StopInput{})
	var test, vet GateState
	for _, g := range free.Snap.Gates {
		switch g.State {
		case Green:
			test = g
		case Red:
			vet = g
		}
	}
	if test.Gate == "" || vet.Gate == "" || !test.Required || !vet.Required || free.Snap.RecordGreen {
		t.Fatalf("premise: a green and a red gate, both required, the record not green: %+v", free.Snap.Gates)
	}
	pinned := stopOf(r, nil, "s1", StopInput{Required: map[string]bool{vet.Gate: false}})
	if len(pinned.Obligations) == 0 {
		t.Fatal("the pinned ledger is empty: the green gate's rows are gone too")
	}
	for _, o := range pinned.Obligations {
		if o.ID != ObligationID(o.Scope, o.Kind, test.Family) {
			t.Errorf("a row %+v of a gate other than the unpinned green one", o)
		}
	}
	if !pinned.Snap.RecordGreen {
		t.Error("the record is not green with its red gate pinned advisory")
	}
}
