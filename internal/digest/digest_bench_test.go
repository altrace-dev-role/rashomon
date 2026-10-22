package digest

import (
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// benchOpenSession writes a session of calls into a pinned store at root and
// opens it the way a digest's caller must, with OpenExisting.
func benchOpenSession(tb testing.TB, root string, calls int) (*store.Store, payload.Turn) {
	tb.Helper()
	turn, err := payload.WriteSession(root, calls, payload.Prompts)
	if err != nil {
		tb.Fatal(err)
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		tb.Fatal(err)
	}
	return st, turn
}

// benchCheckDigest is the work-done check: the digest counted every call the
// generator put in the last turn, matched its executions, named the refused
// call, counted the failed run and read the turn's coverage.
func benchCheckDigest(b *testing.B, d *Digest, turn payload.Turn) {
	b.Helper()
	switch {
	case d == nil:
		b.Fatal("no digest was built")
	case d.PromptID != turn.PromptID:
		b.Fatalf("digest of prompt %q, want the last turn %q", d.PromptID, turn.PromptID)
	case d.Declarations.Recorded != turn.Calls:
		b.Fatalf("digest counted %d calls, want the %d generated in the turn", d.Declarations.Recorded, turn.Calls)
	case d.Executions.Recorded != turn.Executions:
		b.Fatalf("digest counted %d executions, want %d", d.Executions.Recorded, turn.Executions)
	case len(d.Declarations.WithoutExecution) != 1 || d.Declarations.WithoutExecution[0].ToolUseID != turn.RefusedID:
		b.Fatalf("calls without execution %v, want only the refused %s", d.Declarations.WithoutExecution, turn.RefusedID)
	case d.SilentFailures.Failed != turn.Failed:
		b.Fatalf("digest counted %d failed runs, want %d", d.SilentFailures.Failed, turn.Failed)
	case d.Coverage.State != store.StateVerified:
		b.Fatalf("turn coverage %s %v, want verified", d.Coverage.State, d.Coverage.Reasons)
	}
}

// BenchmarkBuild is the Stop-time digest at the real session sizes: the
// median, p90 and largest session measured (188, 899 and 3,488 calls) and
// 20,000. Each size's session is generated once; SetBytes is the store bytes
// the session holds, so MB/s is the read rate.
func BenchmarkBuild(b *testing.B) {
	now := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	for _, calls := range []int{188, 899, 3488, 20000} {
		root := b.TempDir()
		var (
			st   *store.Store
			turn payload.Turn
		)
		b.Run(strconv.Itoa(calls), func(b *testing.B) {
			if st == nil {
				st, turn = benchOpenSession(b, root, calls)
			}
			b.ReportAllocs()
			b.SetBytes(turn.Bytes)
			var (
				d   *Digest
				err error
			)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if d, err = Build(st, payload.Session, "", payload.FinalMessage, now); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			benchCheckDigest(b, d, turn)
		})
	}
}
