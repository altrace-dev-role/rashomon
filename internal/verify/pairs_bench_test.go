package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// benchPairsAt is benchPairsRun's last execution's seq: Evaluate's point at
// the end of the run.
const benchPairsAt = 12000

// benchPairsRun is benchEvalRun's 6000 calls with ten tests_only_then_green
// pairs laid over them: at 300, 900, ..., 5700 a red go test, an edit of a
// test file and a green go test (1.8r's doubt 4, card 1.8r2).
func benchPairsRun(tb testing.TB) *store.Run {
	const n = 6000
	calls := make([]call, 0, n)
	for i := int64(1); i <= n; i++ {
		switch {
		case i%600 == 300:
			calls = append(calls, redRun(i))
		case i%600 == 301:
			calls = append(calls, edit(i, "pkg/a_test.go"))
		case i%600 == 302:
			calls = append(calls, greenRun(i))
		case i%7 == 0:
			calls = append(calls, gateCall(i, "go-test", store.ExecOK))
		case i%11 == 0:
			calls = append(calls, gateCall(i, "go-vet", store.ExecOK))
		default:
			calls = append(calls, edit(i, "pkg/a.go"))
		}
	}
	r := runOf(tb, calls...)
	labelTests(r)
	return r
}

func BenchmarkPairs(b *testing.B) {
	// One literal b.Run name: RES36 reads every b.Run name statically, and a
	// top-level name keeps each segment short of TestH113's entropy test.
	r := benchPairsRun(b)
	_, writes, _, _ := classify(r)
	picked := pickExecutions(r)
	if got := integrity(r, picked, writes, 0, benchPairsAt, ""); len(got.Unresolved) != 10 {
		b.Fatalf("the fixture gives %d unresolved, want the ten pairs", len(got.Unresolved))
	}
	b.Run("ten_6000", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			integrity(r, picked, writes, 0, benchPairsAt, "")
		}
	})
}
