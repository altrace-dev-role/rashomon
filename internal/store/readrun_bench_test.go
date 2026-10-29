package store_test

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// readRunBudget is the lock-wait budget the reads below are given, the steer
// step's; nothing else holds the run's locks here, so each is taken at once.
const readRunBudget = 10 * time.Millisecond

// BenchmarkReadRun is one full read of a run, the read every Stop, every
// failed runner call's step and every report pays (card 2.3b R6): a session
// of 100 and of 6,000 calls from payload.WriteSession, the generator
// BenchmarkBuild reads, through ReadRunConsistent. SetBytes is the session's
// bytes, so MB/s is the read rate. It is package store_test because the
// generator imports store.
func BenchmarkReadRun(b *testing.B) {
	for _, c := range []struct {
		name  string
		calls int
	}{{"calls_100", 100}, {"calls_6000", 6000}} {
		root := b.TempDir()
		var (
			st   *store.Store
			turn payload.Turn
		)
		b.Run(c.name, func(b *testing.B) {
			if st == nil {
				var err error
				if turn, err = payload.WriteSession(root, c.calls, payload.Prompts); err != nil {
					b.Fatal(err)
				}
				if st, err = store.OpenExisting(root); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.SetBytes(turn.Bytes)
			var (
				run *store.Run
				err error
			)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if run, err = st.ReadRunConsistent(payload.Session, readRunBudget); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			// The work-done check, payload's own: every line read, a
			// declaration and a terminal per call, an execution for each but
			// the refused one, two coverage records per call.
			if run.Skipped != 0 || len(run.Declarations) != c.calls || len(run.Terminals) != c.calls ||
				len(run.Executions) != c.calls-1 || len(run.Coverage) != 2*c.calls {
				b.Fatalf("read %d declarations, %d terminals, %d executions, %d coverage, %d skipped; want %d, %d, %d, %d, 0",
					len(run.Declarations), len(run.Terminals), len(run.Executions), len(run.Coverage), run.Skipped,
					c.calls, c.calls, c.calls-1, 2*c.calls)
			}
		})
	}
}
