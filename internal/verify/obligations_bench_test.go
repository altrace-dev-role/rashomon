package verify

import "testing"

// BenchmarkLedger is BenchmarkEvaluate's run_6000 fixture through
// EvaluateWithObligations: what a caller pays for the snapshot and the
// ledger. The view's share is this row less BenchmarkEvaluate/run_6000 from
// the same measure (Gate A Q8). The name is short because budgets.json is a
// fixture H-113 screens, and a run of 20 or more mixed-case letters there,
// "EvaluateWithObligations" among them, reads as a secret's shape.
func BenchmarkLedger(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		name string
		n    int64
	}{{"run_6000", 6000}} {
		r := benchEvalRun(b, c.n)
		last := 2 * c.n // the last call's execution
		if obs := obAt(r, last); len(obs) != 6 {
			b.Fatalf("the fixture gives %d rows, want six: two gates of three", len(obs))
		}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				obAt(r, last)
			}
		})
	}
}
