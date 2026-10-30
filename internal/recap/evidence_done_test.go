package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// withRows is v with its Expected projected into ledger rows, as EvaluateStop
// projects them: the done flag reads each kind's state from its row.
func withRows(v verify.StopView) verify.StopView {
	v.Obligations = make([]verify.Obligation, 0, len(v.Expected))
	for _, e := range v.Expected {
		v.Obligations = append(v.Obligations, verify.Obligation{
			ID:    verify.ObligationID(verify.ScopeSession, verify.ExpectedEvidence, e.Kind),
			Scope: verify.ScopeSession, Kind: verify.ExpectedEvidence, Source: verify.SourceTask, State: e.State,
			Evidence: []int64{},
		})
	}
	return v
}

// doneClaim is a final message's one strict done claim at verdict v.
func doneClaim(v verify.State) verify.ClaimScan {
	return verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(v)}}}
}

// TestEV6_DoneWithUnmetEvidenceNamesTheKind (card 3.3, Gate B 1): a strict
// done claim with expected evidence unmet fires the done flag even on a
// verified row, one sentence per unmet kind after the row's, in the rows'
// order, met, open and unknown rows unnamed; an unmet kind with no done claim
// raises no flag.
func TestEV6_DoneWithUnmetEvidenceNamesTheKind(t *testing.T) {
	green := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	v := viewOf(verify.RowVerified, wrote, green)
	v.Scan = doneClaim(verify.Green)
	v.Expected = []verify.Expected{
		{Kind: "real_data_run", State: verify.ObligationUnmet, Reason: "no gate declares it"},
		{Kind: "cross_check", State: verify.ObligationMet, Gate: "spec:x"},
		{Kind: "judgment_disclosed", State: verify.ObligationUnknown},
		{Kind: "mechanism_stated", State: verify.ObligationUnmet, Reason: "the task file states no mechanism"},
	}
	v = withRows(v)
	row := fitted(v, prefix+doneLead)
	if row == "" {
		t.Fatal("premise: the verified row has a sentence")
	}
	want := prefix + doneLead + row + "." +
		" expected evidence real_data_run is not on the record (no gate declares it)." +
		" expected evidence mechanism_stated is not on the record (the task file states no mechanism).\n" +
		indent + "→ rashomon report --session sess-1"
	if got, ok := DoneFlag(v, "sess-1", false); !ok || got != want {
		t.Errorf("DoneFlag = %q, %v\nwant %q, true", got, ok, want)
	}
	for _, scan := range []verify.ClaimScan{
		{Claims: []verify.ClaimVerdict{}},
		{Claims: []verify.ClaimVerdict{{Class: verify.ClaimLintClean, Strict: true, Verdict: verify.Verdict(verify.Green)}}},
		{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: false, Verdict: verify.Verdict(verify.Green)}}},
	} {
		nd := v
		nd.Scan = scan
		if got, ok := DoneFlag(nd, "sess-1", false); ok || got != "" {
			t.Errorf("no strict done claim %+v: DoneFlag = %q, %v; want no flag", scan.Claims, got, ok)
		}
	}
}

// TestEV7_DoneWithAllMetSaysNothingNew: with every expected row met, open or
// unknown, the done flag is what it was before 3.3: none on a verified row
// with a green claim, and the row's sentence alone on a red one.
func TestEV7_DoneWithAllMetSaysNothingNew(t *testing.T) {
	ex := []verify.Expected{
		{Kind: "real_data_run", State: verify.ObligationMet, Gate: "spec:r"},
		{Kind: "judgment_disclosed", State: verify.ObligationOpen},
		{Kind: "mechanism_stated", State: verify.ObligationUnknown},
	}
	green := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	v := viewOf(verify.RowVerified, wrote, green)
	v.Scan, v.Expected = doneClaim(verify.Green), ex
	v = withRows(v)
	if got, ok := DoneFlag(v, "sess-1", false); ok || got != "" {
		t.Errorf("verified, all met: DoneFlag = %q, %v; want no flag", got, ok)
	}
	red := gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1))
	rv := viewOf(verify.RowRed, wrote, red)
	rv.Scan, rv.Expected = scanOf("All 42 tests pass.", "", red), ex
	rv = withRows(rv)
	want := prefix + "done was claimed; the record shows 2 files edited; go exited 1 after the last edit (call 7).\n" +
		indent + "→ rashomon report --session sess-1"
	if got, ok := DoneFlag(rv, "sess-1", false); !ok || got != want {
		t.Errorf("red, all met: DoneFlag = %q, %v\nwant %q, true", got, ok, want)
	}
}

// TestLW1k_DoneClauseReadsTheLedger (card LEDGER-WIRE1): the done flag's
// expected clause takes each kind's state from its ledger row and its words
// from Expected, so it cannot disagree with the ledger: Expected unmet over a
// met row names nothing; Expected met over an unmet row names the kind, with
// Expected's reason.
func TestLW1k_DoneClauseReadsTheLedger(t *testing.T) {
	green := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	v := viewOf(verify.RowVerified, wrote, green)
	v.Scan = doneClaim(verify.Green)
	v.Expected = []verify.Expected{{Kind: "cross_check", State: verify.ObligationUnmet, Reason: "no gate declares it"}}
	v = withRows(v)
	v.Obligations[0].State = verify.ObligationMet
	if got, ok := DoneFlag(v, "sess-1", false); ok || got != "" {
		t.Errorf("Expected unmet over a met row: DoneFlag = %q, %v; want no flag", got, ok)
	}
	v.Expected = []verify.Expected{{Kind: "cross_check", State: verify.ObligationMet, Reason: "no gate declares it"}}
	v.Obligations[0].State = verify.ObligationUnmet
	want := prefix + doneLead + fitted(v, prefix+doneLead) + "." +
		" expected evidence cross_check is not on the record (no gate declares it).\n" +
		indent + "→ rashomon report --session sess-1"
	if got, ok := DoneFlag(v, "sess-1", false); !ok || got != want {
		t.Errorf("Expected met over an unmet row: DoneFlag = %q, %v\nwant %q, true", got, ok, want)
	}
}
