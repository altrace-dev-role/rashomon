package verify

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ledgerAt is the ledger at a stop point at seq with no index and no spec,
// built as EvaluateStop builds it, by evaluate and the ledger's one producer,
// without the Stop view's other parts.
func ledgerAt(r *store.Run, seq int64) []Obligation {
	var c classified
	snap := evaluate(r, nil, nil, nil, nil, Point{Kind: PointStop, Seq: seq, SessionID: "s1"}, time.Time{}, &c)
	return ledgerOf(snap, &c, expected(snap, evidenceIn{atStop: true}))
}

// BenchmarkLedger is BenchmarkEvaluate's run_6000 fixture through ledgerAt:
// what the Stop view pays for the snapshot and the ledger. The view's share is
// this row less BenchmarkEvaluate/run_6000 from the same measure (Gate A Q8).
// The name is short because budgets.json is a fixture H-113 screens, and a
// run of 20 or more mixed-case letters there reads as a secret's shape.
func BenchmarkLedger(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		name string
		n    int64
	}{{"run_6000", 6000}} {
		r := benchEvalRun(b, c.n)
		last := 2 * c.n // the last call's execution
		if obs := ledgerAt(r, last); len(obs) != 6 {
			b.Fatalf("the fixture gives %d rows, want six: two gates of three", len(obs))
		}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				ledgerAt(r, last)
			}
		})
	}
}
