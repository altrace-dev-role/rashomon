package verify

// The predicates a requirement row is checked by (W-M3.4): a file exists, a
// symbol exists, a gate ran green, a test of that name exists.
const (
	PredFileExists   = "file_exists"
	PredSymbolExists = "symbol_exists"
	PredGateGreen    = "gate_green"
	PredTestNamed    = "test_named"
)

// Preds is the closed list of requirement predicates, in the order above.
var Preds = []string{PredFileExists, PredSymbolExists, PredGateGreen, PredTestNamed}

// SourceRequirement is the source of a requirement's row. The row is an
// expected_evidence row, as a task's is, told apart by its source (a task's is
// SourceTask) and keyed by ReqFamily.
const SourceRequirement = "requirement"

// ReqRow is one requirement the Stop's caller checked (W-M3.4): its id, its
// predicate, one of Preds, and the predicate's argument. Known false is a
// check that could not be made, a row unknown and never met (INV-2); Holds is
// read only where Known.
type ReqRow struct {
	ID, Pred, Arg string
	Holds, Known  bool
}

// ReqFamily is the family a requirement's row is keyed by: "req:" and the
// requirement's id. The row's id is ObligationID(scope, ExpectedEvidence,
// ReqFamily(id)); no task evidence kind starts "req:", so a requirement never
// takes a task row's id.
func ReqFamily(id string) string { return "req:" + id }

// reqObligations is one expected_evidence row per requirement id in reqs, in
// scope, the first row of an id kept: a second would take the same row id.
// Each row is of SourceRequirement, keyed by ReqFamily, its id in Req, no
// evidence cited, its state reqState's. nil for no reqs. EvaluateStop appends
// the rows after the whole ledger, so no other row moves, and UnmetExpected,
// which matches task kinds only, never reads one.
func reqObligations(scope string, snap *Snapshot, reqs []ReqRow) []Obligation {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]Obligation, 0, len(reqs))
next:
	for _, q := range reqs {
		for i := range out {
			if out[i].Req == q.ID {
				continue next
			}
		}
		out = append(out, Obligation{ID: ObligationID(scope, ExpectedEvidence, ReqFamily(q.ID)), Scope: scope,
			Kind: ExpectedEvidence, Source: SourceRequirement, State: reqState(snap, q), Evidence: []int64{}, Req: q.ID})
	}
	return out
}

// reqState is q's row state at snap. A gate_green requirement reads the
// Stop's own row for the gate its argument names, the row whose idFamily it
// equals ("spec:" and the id for a declared gate), and never the caller's
// Holds or Known. Its state is the one gateAfterWriteTable gives that gate,
// looked up and not restated, so the requirement row and the gate's own row
// of one ledger never disagree: green is met; red and stale are unmet;
// absent and backgrounded are open, not seen done, with or without a write;
// masked_inconclusive and unknown are unknown, and so is a state the table
// does not hold. No row of that id is unknown, and so is a row whose own
// ledger rows read unknown (obligations: an incomplete record, a source
// outside the four), so a requirement never reads met where its gate's rows
// cannot say (INV-2). Every other predicate is the caller's check: unknown
// where it could not be made, met where it holds, else unmet.
func reqState(snap *Snapshot, q ReqRow) ObligationState {
	if q.Pred != PredGateGreen {
		switch {
		case !q.Known:
			return ObligationUnknown
		case q.Holds:
			return ObligationMet
		}
		return ObligationUnmet
	}
	if snap.Recording != Complete {
		return ObligationUnknown
	}
	for i := range snap.Gates {
		g := &snap.Gates[i]
		if idFamily(g) != q.Arg {
			continue
		}
		if _, known := obligationSource(g.Source); !known {
			return ObligationUnknown
		}
		for _, r := range gateAfterWriteTable {
			if r.in == g.State {
				return r.out
			}
		}
		return ObligationUnknown
	}
	return ObligationUnknown
}
