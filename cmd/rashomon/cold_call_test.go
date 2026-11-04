package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestColdCall measures the allocation of exactly one hook or post call in a
// freshly started test process: runtime.ReadMemStats brackets a single
// cmdHook/cmdPost call against a just-opened store, and the TotalAlloc delta
// is printed to real stdout (fmt.Printf, not t.Log) as "COLD <id> <bytes>\n"
// for a harness to parse from a subprocess's CombinedOutput. `go test`
// itself still buffers a passing test's stdout and only ever shows it with
// -v or on failure (test/perf/cold_test.go's child invocation passes -v for
// exactly that reason) -- fmt.Printf only avoids t.Log's own, separate
// buffering, which needs -v regardless of how the parent process is run.
//
// This is deliberately a *testing.T, not a *testing.B: a cold figure is a
// property of a fresh process, not of a steady-state loop, and running it
// b.N times (even once) inside the testing.B machinery would still share
// whatever the Go runtime warms up across ResetTimer -- the harness instead
// spawns a new "go test -run" process per entry (test/perf/cold_test.go) so
// every id gets its own process.
//
// Subtests are named exactly the budgets.json entry id so the harness's
// -run regex can select one by name. recap.* and probe.* are not covered
// here (see test/perf/cold_test.go's comment).
func TestColdCall(t *testing.T) {
	for _, c := range []struct {
		id      string
		entry   func([]string, io.Reader, io.Writer) int
		payload func(testing.TB) []byte
	}{
		{"hook.bash_short", cmdHook, benchBashShortPre},
		{"hook.edit_50k", cmdHook, benchEdit50kPre},
		{"post.bash_short", postEntry, benchBashShortPost},
		{"post.edit_314k", postEntry, benchEdit314kPost},
	} {
		t.Run(c.id, func(t *testing.T) {
			// One P for the whole subtest, its setup included (PERF-EDGE1):
			// json/v2 keeps its decoders in a sync.Pool, whose private slot is
			// per P, so a call that ran on another P than its setup missed the
			// pool and read a second level 5 to 8K above the first
			// (test/perf's TestPE1_ColdCallIsOneLevel). The pin is the
			// measurement's: hook and post leave GOMAXPROCS as the runtime set
			// it (85.3.8, test/perf/gate.go), and no rashomon code reads
			// GOMAXPROCS or NumCPU.
			defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
			var g0 runtime.MemStats
			runtime.ReadMemStats(&g0)
			ms0, _ := runtime.ThreadCreateProfile(nil)
			root, installID := benchStarted(t)
			in := newBenchInput(t, c.payload(t))
			args := []string{install.Marker, installID}
			var stderr benchSink
			coldWarmThreads(coldIdleThreads)

			var m0, m1 runtime.MemStats
			ms1, _ := runtime.ThreadCreateProfile(nil)
			runtime.ReadMemStats(&m0)
			if code := c.entry(args, in.next(0), &stderr); code != exitOK {
				t.Fatalf("entry exited %d", code)
			}
			runtime.ReadMemStats(&m1)
			ms2, _ := runtime.ThreadCreateProfile(nil)

			if stderr.writes != 0 {
				t.Fatalf("stderr: %q", stderr.lastWrite())
			}

			delta := m1.TotalAlloc - m0.TotalAlloc
			fmt.Printf("COLD %s %d\n", c.id, delta)
			// The OS threads the runtime started in setup and in the call,
			// the witness of coldWarmThreads: test/perf's
			// TestPE1_ColdCallIsOneLevel requires call=0.
			fmt.Printf("COLDTHREADS %s setup=%d call=%d\n", c.id, ms1-ms0, ms2-ms1)
			// The GC cycles that began in setup and in the call, and the
			// heap against its goal before the call (PERF-EDGE1 probe): a
			// GC start moves sync.Pool's primary cache to its victim cache
			// and a second drops it, so two between setup's Put and the
			// call's Get send json/v2's decoder Get to New.
			fmt.Printf("COLDGC %s setup=%d call=%d heap=%d next=%d\n", c.id, m0.NumGC-g0.NumGC, m1.NumGC-m0.NumGC, m0.HeapAlloc, m0.NextGC)
			// The call's objects by size class, and the run's lock-wait
			// note (PERF-EDGE1 probe): a retry inside the call reads as a
			// few small objects that add up to the step, a fresh buffer as
			// one object of about its size, and an append that polled for
			// its lock as a note (store's noteLockWait).
			classes, rest, small := coldSizeClasses(&m0, &m1)
			fmt.Printf("COLDSIZE %s mallocs=%d tiny+large=%d largebytes=%d lockwait=%s classes=%s\n", c.id, m1.Mallocs-m0.Mallocs, rest, int64(delta)-int64(small), coldLockWait(root), classes)
		})
	}
}

// coldIdleThreads is how many goroutines coldWarmThreads locks to threads at
// once: the macOS runner's calls started one or two Ms (PERF-EDGE1 probes).
const coldIdleThreads = 3

// coldWarmThreads leaves the runtime idle Ms for the call (PERF-EDGE1), as
// setup's probe start leaves json/v2's decoder in its pool. At one P, sysmon
// hands the P of a syscall that outlasts its tick to another M, and when
// none is idle the runtime allocates one: on darwin/arm64 5320 bytes (its m,
// g0 and gsignal, two profiling stacks, a weak pointer), the macOS runner's
// second level. n goroutines hold n threads at once, then unlock and return;
// their Ms park idle, the runtime frees no idle M, and a handoff in the call
// takes one instead of allocating.
func coldWarmThreads(n int) {
	var locked, done sync.WaitGroup
	release := make(chan struct{})
	locked.Add(n)
	done.Add(n)
	for range n {
		go func() {
			defer done.Done()
			runtime.LockOSThread()
			locked.Done()
			<-release
			runtime.UnlockOSThread()
		}()
	}
	locked.Wait()
	close(release)
	done.Wait()
}

// coldSizeClasses is the call's mallocs by size class, m1 against m0: size:count
// for every class whose count moved, the mallocs no class counts (tiny objects,
// which share 16-byte blocks, and objects over the largest class), and the
// bytes the classes account for, so TotalAlloc's delta less them is the large
// objects' bytes.
func coldSizeClasses(m0, m1 *runtime.MemStats) (classes string, rest, small uint64) {
	var b strings.Builder
	var n uint64
	for i := range m1.BySize {
		d := m1.BySize[i].Mallocs - m0.BySize[i].Mallocs
		if d == 0 {
			continue
		}
		n += d
		small += d * uint64(m1.BySize[i].Size)
		fmt.Fprintf(&b, " %d:%d", m1.BySize[i].Size, d)
	}
	return strings.TrimSpace(b.String()), m1.Mallocs - m0.Mallocs - n, small
}

// coldLockWait is the lock-wait note of each run under root, the milliseconds
// its appends waited for their locks, which an append writes only when it
// waited; "none" when no run has one.
func coldLockWait(root string) string {
	notes, _ := filepath.Glob(filepath.Join(root, store.DirRuns, "*", store.FileLockWait))
	var got []string
	for _, p := range notes {
		b, err := os.ReadFile(p)
		if err != nil {
			got = append(got, "unread")
			continue
		}
		got = append(got, strings.TrimSpace(string(b)))
	}
	if len(got) == 0 {
		return "none"
	}
	return strings.Join(got, ",")
}
