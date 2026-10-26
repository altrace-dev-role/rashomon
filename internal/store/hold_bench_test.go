package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// heldAppend is one ordered append, as appendOrdered makes it, that reports
// how long it held the records lock: the time writeOrderedLocked, the
// critical section itself, took.
func heldAppend(st *Store, d Declaration) (time.Duration, error) {
	dir := st.RunDir(d.SessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(filepath.Join(dir, FileRecords), appendFlags, fileMode)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	unlock, err := st.lockAppend(f, lockBudget)
	if err != nil {
		return 0, err
	}
	defer unlock()
	start := time.Now()
	err = st.writeOrderedLocked(f, dir, func(seq int64) any {
		d.Seq = seq
		return d
	})
	return time.Since(start), err
}

// reportHolds reports the p50, p90 and p99 of holds, in milliseconds.
func reportHolds(b *testing.B, holds []time.Duration) {
	if len(holds) == 0 {
		return
	}
	slices.Sort(holds)
	for _, p := range []int{50, 90, 99} {
		at := holds[min(len(holds)-1, len(holds)*p/100)]
		b.ReportMetric(float64(at.Microseconds())/1000, fmt.Sprintf("hold-p%d-ms", p))
	}
}

// BenchmarkOrderedAppendHold measures how long an ordered append holds the
// records lock, the figure every lock budget is derived from (lockbudget.go,
// digest's readLockBudget). Each op is one hook's declaration on its own
// Store, flushed outside the lock as a hook flushes it; "parallel" makes it
// a 64-way burst on one session, which is where holds queue. A "full" store
// syncs in full inside the lock, as an undeferred store does everywhere and
// as every append does on Linux and Windows, which have no barrier
// (sync_other.go); a "deferred" one puts a barrier there instead (REC-34),
// which only darwin has. The holds are reported as hold-p50-ms, hold-p90-ms
// and hold-p99-ms; an append that waited out the lock budget holds nothing
// and is counted as timeouts/op instead, as in
// BenchmarkAppendDeclarationParallel.
func BenchmarkOrderedAppendHold(b *testing.B) {
	for _, c := range []struct {
		name     string
		ways     int
		deferred bool
	}{
		{"serial_full", 1, false},
		{"serial_deferred", 1, true},
		{"parallel_full", burstWays, false},
		{"parallel_deferred", burstWays, true},
	} {
		b.Run(c.name, func(b *testing.B) {
			root := b.TempDir()
			st, err := Open(root)
			if err != nil {
				b.Fatal(err)
			}
			calls := benchCalls(st, benchSpread)
			var (
				mu       sync.Mutex
				holds    []time.Duration
				timeouts int
			)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				errs := make([]error, c.ways)
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
						if c.deferred {
							one.DeferDurability()
						}
						call := calls[(i*c.ways+w)%benchSpread]
						d := call.decl
						d.ToolUseID, d.RecordedAtMS = fmt.Sprintf("toolu_01%s%016d%06d", call.idPad, i, w), benchStartMS+int64(i)
						held, err := heldAppend(one, d)
						if err == nil {
							err = one.Flush()
						}
						mu.Lock()
						if errors.Is(err, ErrLockTimeout) {
							timeouts++
							err = nil
						} else {
							holds = append(holds, held)
						}
						mu.Unlock()
						errs[w] = err
					}(w)
				}
				wg.Wait()
				for _, err := range errs {
					if err != nil {
						b.Fatal(err)
					}
				}
			}
			b.StopTimer()
			if _, landed := benchLines(b, filepath.Join(st.RunDir(benchSession), FileRecords)); landed+timeouts != c.ways*b.N {
				b.Fatalf("%d declarations landed and %d timed out of %d", landed, timeouts, c.ways*b.N)
			}
			reportHolds(b, holds)
			b.ReportMetric(float64(timeouts)/float64(b.N), "timeouts/op")
		})
	}
}
