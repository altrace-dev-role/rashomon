package main

import (
	"fmt"
	"io"
	"runtime"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
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
			_, installID := benchStarted(t)
			in := newBenchInput(t, c.payload(t))
			args := []string{install.Marker, installID}
			var stderr benchSink

			var m0, m1 runtime.MemStats
			runtime.ReadMemStats(&m0)
			if code := c.entry(args, in.next(0), &stderr); code != exitOK {
				t.Fatalf("entry exited %d", code)
			}
			runtime.ReadMemStats(&m1)

			if stderr.writes != 0 {
				t.Fatalf("stderr: %q", stderr.lastWrite())
			}

			delta := m1.TotalAlloc - m0.TotalAlloc
			fmt.Printf("COLD %s %d\n", c.id, delta)
			// The GC cycles that began in setup and in the call, and the
			// heap against its goal before the call (PERF-EDGE1 probe): a
			// GC start moves sync.Pool's primary cache to its victim cache
			// and a second drops it, so two between setup's Put and the
			// call's Get send json/v2's decoder Get to New.
			fmt.Printf("COLDGC %s setup=%d call=%d heap=%d next=%d\n", c.id, m0.NumGC-g0.NumGC, m1.NumGC-m0.NumGC, m0.HeapAlloc, m0.NextGC)
		})
	}
}
