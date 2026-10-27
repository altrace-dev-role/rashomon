package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// benchEvalRun is n calls in two gate families: go-test every seventh call,
// go-vet every eleventh, an edit otherwise (the plan's fixture, with a second
// family).
func benchEvalRun(tb testing.TB, n int64) *store.Run {
	calls := make([]call, 0, n)
	for i := int64(1); i <= n; i++ {
		switch {
		case i%7 == 0:
			calls = append(calls, gateCall(i, "go-test", store.ExecOK))
		case i%11 == 0:
			calls = append(calls, gateCall(i, "go-vet", store.ExecOK))
		default:
			calls = append(calls, edit(i, "pkg/a.go"))
		}
	}
	return runOf(tb, calls...)
}

func BenchmarkEvaluate(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		name string
		n    int64
	}{{"run_100", 100}, {"run_6000", 6000}} {
		r := benchEvalRun(b, c.n)
		last := 2 * c.n // the last call's execution
		if snap := evalAt(r, last); snap.Recording != Complete || len(snap.Gates) != 2 {
			b.Fatalf("the fixture gives %s with %d gates, want complete with two", snap.Recording, len(snap.Gates))
		}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				evalAt(r, last)
			}
		})
	}
}
