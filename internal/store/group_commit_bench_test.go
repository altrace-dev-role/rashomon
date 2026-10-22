package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// burstWays is how many hooks a BenchmarkAppendDeclarationParallel burst
// runs at once: RES-30's and H-16's width.
const burstWays = 64

// hookStoreSequence is the store work of one PreToolUse invocation, as
// cmdHook does it: open the store, defer durability when asked to, append
// the declaration, its terminal and the call coverage record, and flush.
func hookStoreSequence(root string, c benchCall, id string, at int64, deferred bool) error {
	st, err := Open(root)
	if err != nil {
		return err
	}
	if deferred {
		st.DeferDurability()
	}
	c.decl.ToolUseID, c.decl.RecordedAtMS = id, at
	c.term.ToolUseID, c.term.RecordedAtMS = id, at
	c.cov.Phase, c.cov.RecordedAtMS = PhaseCall, at
	if err := st.AppendDeclaration(c.decl); err != nil {
		return err
	}
	if err := st.AppendTerminal(c.term); err != nil {
		return err
	}
	if err := st.AppendCoverage(c.cov); err != nil {
		return err
	}
	return st.Flush()
}

// BenchmarkHookStoreSequence is one hook's store work with a full sync per
// append against group commit's barriers and one flush (REC-34; the A1
// prototype measured 15.0 to 3.0 ms on an Apple M3). The work-done check
// reads back every record each variant wrote.
func BenchmarkHookStoreSequence(b *testing.B) {
	for _, c := range []struct {
		name     string
		deferred bool
	}{{"full", false}, {"deferred", true}} {
		b.Run(c.name, func(b *testing.B) {
			root := b.TempDir()
			st, err := Open(root)
			if err != nil {
				b.Fatal(err)
			}
			call := newBenchCall(st)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := fmt.Sprintf("toolu_01%022d", i)
				if err := hookStoreSequence(root, call, id, benchStartMS+int64(i), c.deferred); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			run, err := st.ReadRun(benchSession)
			if err != nil {
				b.Fatal(err)
			}
			if len(run.Declarations) != b.N || len(run.Terminals) != b.N || len(run.Coverage) != b.N || run.Skipped != 0 {
				b.Fatalf("read back %d declarations, %d terminals, %d coverage, %d skipped after %d hooks",
					len(run.Declarations), len(run.Terminals), len(run.Coverage), run.Skipped, b.N)
			}
		})
	}
}

// BenchmarkAppendDeclarationParallel is one 64-way burst per op: burstWays
// hooks on one session at once, each opening its own store, as each hook
// process does, deferring durability, appending its declaration and
// flushing. The burst is the operation so that its width does not depend on
// GOMAXPROCS or b.N. A declaration that waited out the lock budget is
// counted as timeouts/op, the figure H-16 turns red on (archive 85.3.13);
// the work-done check is that every declaration either landed or timed out.
func BenchmarkAppendDeclarationParallel(b *testing.B) {
	root := b.TempDir()
	st, err := Open(root)
	if err != nil {
		b.Fatal(err)
	}
	call := newBenchCall(st)
	timeouts := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errs := make([]error, burstWays)
		var wg sync.WaitGroup
		for w := range errs {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				one, err := Open(root)
				if err != nil {
					errs[w] = err
					return
				}
				one.DeferDurability()
				d := call.decl
				d.ToolUseID, d.RecordedAtMS = fmt.Sprintf("toolu_01%016d%06d", i, w), benchStartMS+int64(i)
				if err := one.AppendDeclaration(d); err != nil {
					errs[w] = err
					return
				}
				errs[w] = one.Flush()
			}(w)
		}
		wg.Wait()
		for _, err := range errs {
			switch {
			case errors.Is(err, ErrLockTimeout):
				timeouts++
			case err != nil:
				b.Fatal(err)
			}
		}
	}
	b.StopTimer()
	_, landed := benchLines(b, filepath.Join(st.RunDir(benchSession), FileRecords))
	if landed+timeouts != burstWays*b.N {
		b.Fatalf("%d declarations landed and %d timed out of %d", landed, timeouts, burstWays*b.N)
	}
	b.ReportMetric(float64(timeouts)/float64(b.N), "timeouts/op")
}
