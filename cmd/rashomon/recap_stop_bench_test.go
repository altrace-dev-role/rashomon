package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// BenchmarkRecapStop is the Stop view's cost per turn (card 2.4 R14) on a
// payload.WriteSession run of 100 and 6,000 calls: the one read of the run the
// digest shares, EvaluateStop with the claim scan over the final message, Cut,
// the index read, Observe and WriteIndex under the payload's cwd, one append
// and the Flush barrier. The digest over the same read is BenchmarkBuild's
// row; it is built once, before the timer, for the prompt it chooses. Each op
// appends one checkpoint, which the next op's read passes over uncounted.
func BenchmarkRecapStop(b *testing.B) {
	now := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name  string
		calls int
	}{{"run_100", 100}, {"run_6000", 6000}} {
		b.Run(c.name, func(b *testing.B) {
			root := b.TempDir()
			if _, err := payload.WriteSession(root, c.calls, payload.Prompts); err != nil {
				b.Fatal(err)
			}
			st, err := store.OpenExisting(root)
			if err != nil {
				b.Fatal(err)
			}
			run, err := digest.ReadRun(st, payload.Session)
			if err != nil {
				b.Fatal(err)
			}
			d, err := digest.BuildFrom(run, st, payload.Session, "", payload.FinalMessage, now)
			if err != nil {
				b.Fatal(err)
			}
			p := recapPayload{SessionID: payload.Session, LastAssistantMessage: payload.FinalMessage,
				Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: payload.CWD}
			var view stopTurn
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				r, err := digest.ReadRun(st, payload.Session)
				if err != nil {
					b.Fatal(err)
				}
				if view, err = stopView(st, r, d, p, false, now); err != nil {
					b.Fatal(err)
				}
				if err := st.Flush(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			benchCheckStop(b, st, view)
		})
	}
}

// benchCheckStop is the work-done check: the view classified the run's gates
// and read the final message's claim, every op appended its checkpoint, and
// the index holds the run's families.
func benchCheckStop(b *testing.B, st *store.Store, view stopTurn) {
	b.Helper()
	if !view.speaks || len(view.view.Snap.Gates) == 0 || len(view.view.Scan.Claims) == 0 {
		b.Fatalf("the view spoke %v over %d gates and %d claims, want both read", view.speaks,
			len(view.view.Snap.Gates), len(view.view.Scan.Claims))
	}
	idx, err := verify.ReadIndex(verify.IndexPath(st.Root(), view.view.Index.ProjectKey))
	if err != nil || idx == nil || len(idx.Entries) == 0 {
		b.Fatalf("index %+v (%v), want the run's families", idx, err)
	}
	if n := benchCheckpoints(b, st); n != b.N {
		b.Fatalf("%d checkpoints after %d ops", n, b.N)
	}
}

// benchCheckpoints counts the run's checkpoint lines.
func benchCheckpoints(b *testing.B, st *store.Store) int {
	b.Helper()
	data, err := os.ReadFile(filepath.Join(st.RunDir(payload.Session), store.FileRecords))
	if err != nil {
		b.Fatal(err)
	}
	return bytes.Count(data, []byte(`{"type":"verify",`))
}
