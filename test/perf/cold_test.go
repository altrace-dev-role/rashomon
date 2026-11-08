//go:build perf

package perf

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// coldToleranceFraction is the margin ColdAllocBytesMax is gated at, the same
// flat ten percent AllocBytesMax's benchmark rows use (TestRES36f_); unlike
// the RSS rows, the figure may be per-platform: an entry carries its own
// platform's figure in budgets.json's platforms.<plat>.cold_alloc_bytes_max
// (ColdBudget) where that platform reads away from the top-level one, which
// stays the fallback for a platform with no figure of its own. The tolerance
// itself is the same flat ten percent on every platform.
const coldToleranceFraction = 0.10

// coldRuns is how many fresh processes coldCallAlloc spawns per entry
// before gating: a single cold measurement carries normal process-start
// noise (GC pacing, OS scheduling) that can eat most of the 10% margin by
// itself, so the gate compares the minimum of coldRuns runs, the same
// "best of three" the budgets.json basis for every cold_alloc_bytes_max
// entry already claims -- gating a single run would silently be stricter
// than the figure it is checked against.
const coldRuns = 3

// TestColdAllocBudget gates budgets.json's cold_alloc_bytes_max entries
// against a real measurement: for each entry whose field is nonzero, it
// spawns coldRuns fresh `go test -run` processes against cmd/rashomon's
// TestColdCall (one process per run, so the figure is a genuinely cold one,
// not a second subtest sharing an earlier one's warmed process state),
// reads the one "COLD <id> <bytes>" line each subtest prints to real
// stdout, and gates the minimum of the coldRuns.
//
// Entries whose cold_alloc_bytes_max is 0 (recap.*, probe.*, and anything
// with no benchmark row) are skipped: that 0 means "not yet measured" per
// the field's own doc comment, not a budget of zero. Every entry this test
// gates marks ColdGate(id) run (runs.go), so a -run pattern that stops
// selecting this test -- or an entry a future change starts measuring --
// is caught by RASHOMON_PERF_REQUIRE_ALL, not silently skipped.
//
// -v is required on the child's `go test` invocation: go test buffers a
// passing test's stdout and only ever shows it with -v or on failure (see
// TestColdCall's own comment in cmd/rashomon/cold_call_test.go).
func TestColdAllocBudget(t *testing.T) {
	bud := loadBudgets(t)
	overrides := bud.Platforms[platform()].ColdAllocBytesMax
	for _, e := range bud.Entries {
		if e.ColdAllocBytesMax <= 0 {
			continue
		}
		e := e
		budgetBytes := ColdBudget(e, overrides)
		t.Run(e.ID, func(t *testing.T) {
			ran[ColdGate(e.ID)] = true
			measured, all := coldCallAlloc(t, e.ID)
			if OverBudget(float64(measured), float64(budgetBytes), coldToleranceFraction) {
				t.Errorf("%s: cold call allocated %d bytes (runs %v), over budget %d (+%.0f%%)", e.ID, measured, all, budgetBytes, coldToleranceFraction*100)
				return
			}
			t.Logf("%s: cold call allocated %d bytes (runs %v) against budget %d (+%.0f%%)", e.ID, measured, all, budgetBytes, coldToleranceFraction*100)
		})
	}
}

// coldLevelRuns is how many fresh processes TestPE1_ColdCallIsOneLevel
// spawns, and coldLevelGapBytes the largest step their sorted figures may
// take (LevelGap). The two levels an unpinned process draws sit 3.7K or more
// apart (pe1-gmp), and one high run read 4808 B over nine low ones
// (pe1-cold2). The figure's own noise opens no step that wide but does widen
// its range: at 29ee28f9, 120 fresh processes made 14 to 26 of sync.Map's
// 160-byte trie nodes in the call while the rest of the figure took two
// values 128 B apart, and over nineteen CI sets of ten the largest step read
// 944 B while the range read up to 2064 B (run 42447803644, linux), so the
// gate reads the step, not the range.
const (
	coldLevelRuns     = 10
	coldLevelGapBytes = 2048
)

// TestPE1_ColdCallIsOneLevel (card PERF-EDGE1): the cold figure is one
// level, not two. A fresh process's post call either found json/v2's pooled
// decoder on the P it ran on or missed it and paid a new decoder's string
// cache, so at GOMAXPROCS=3 eleven of twenty runs read 153.3K to 154.7K and
// nine read 158.4K to 161.5K (pe1-gmp; CI macos read [163256 156312
// 156152]), and TestColdAllocBudget's minimum of three went red whenever all
// three drew the high level. TestColdCall pins one P for its subtest and
// coldCallAllocOnce starts its process at one P, so ten fresh runs take no
// step over coldLevelGapBytes. Break: both pins dropped, and ten runs draw
// both levels; the subtest's pin alone still drew one high run in ten at
// eight P (pe1-cold2: 160784 beside nine at 155144 to 155976). A call during
// which the runtime started an OS thread read 5.3K or 10.6K over the rest on
// the macOS runner (5320 bytes per new M), so TestColdCall's setup leaves
// idle Ms (coldWarmThreads) and this test requires the call to start none.
func TestPE1_ColdCallIsOneLevel(t *testing.T) {
	const id = "post.bash_short"
	all := make([]int64, coldLevelRuns)
	threads := make([]int, coldLevelRuns)
	for i := range all {
		all[i], threads[i] = coldCallAllocOnce(t, id)
	}
	if slices.Max(threads) > 0 {
		t.Errorf("%s: the runtime started OS threads inside the call (runs %v, threads %v), want none: TestColdCall's setup leaves idle Ms (coldWarmThreads)", id, all, threads)
	}
	lo, hi := slices.Min(all), slices.Max(all)
	gap, below, above, _ := LevelGap(all)
	if gap > coldLevelGapBytes {
		t.Errorf("%s: %d fresh cold calls step %d bytes from %d to %d (range %d to %d, runs %v), want no step over %d: the cold figure has more than one level", id, len(all), gap, below, above, lo, hi, all, coldLevelGapBytes)
		return
	}
	t.Logf("%s: %d fresh cold calls, largest step %d bytes (range %d to %d, runs %v)", id, len(all), gap, lo, hi, all)
}

// coldCallAlloc runs cmd/rashomon's TestColdCall/<id> subtest in coldRuns
// fresh `go test` processes and returns the minimum of the TotalAlloc
// deltas each one printed, and every run's figure for the failure message.
func coldCallAlloc(t *testing.T, id string) (int64, []int64) {
	t.Helper()
	all := make([]int64, coldRuns)
	for i := range all {
		all[i], _ = coldCallAllocOnce(t, id)
	}
	min := all[0]
	for _, n := range all[1:] {
		if n < min {
			min = n
		}
	}
	return min, all
}

// coldCallAllocOnce runs cmd/rashomon's TestColdCall/<id> subtest in its
// own `go test` process and returns the one TotalAlloc delta it printed and
// the OS threads the runtime started inside the call (COLDTHREADS call=).
// The process starts at one P (PERF-EDGE1): TestColdCall's pin holds from
// its subtest's first line, but the runtime and the testing package start at
// every P the machine has, and a start at one P, GOMAXPROCS=1 in the
// environment, drew no high run in thirty (pe1-gmp, pe1-cold2). -exec starts
// the test binary through env, so the go command still builds at the
// machine's P.
func coldCallAllocOnce(t *testing.T, id string) (int64, int) {
	t.Helper()
	pattern := "TestColdCall/" + regexp.QuoteMeta(id)
	cmd := exec.Command("go", "test", "-exec", "env GOMAXPROCS=1", "-run", pattern, "-v", "-count=1", "./cmd/rashomon")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -run %s ./cmd/rashomon: %v\n%s", pattern, err, out)
	}
	n, found := parseColdLine(out, id)
	if !found {
		t.Fatalf("go test -run %s ./cmd/rashomon printed no %q line:\n%s", pattern, "COLD "+id, out)
	}
	threads := coldProbeLine(out, "COLDTHREADS", id)
	t.Logf("cold %s: %d bytes; %s; %s; %s", id, n, threads, coldProbeLine(out, "COLDGC", id), coldProbeLine(out, "COLDSIZE", id))
	var setup, call int
	if _, err := fmt.Sscanf(threads, "COLDTHREADS "+id+" setup=%d call=%d", &setup, &call); err != nil {
		t.Fatalf("%s: %q: %v", id, threads, err)
	}
	return n, call
}

// coldProbeLine returns the "<tag> <id> ..." line TestColdCall prints for id
// beside its COLD line (PERF-EDGE1 probe: COLDGC, the GC cycles that began
// around the call; COLDSIZE, its objects by size class), so each figure is
// logged with them.
func coldProbeLine(out []byte, tag, id string) string {
	prefix := tag + " " + id + " "
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return "no " + tag + " line"
}

// parseColdLine finds the one "COLD <id> <bytes>" line TestColdCall prints
// for id and returns its byte count.
func parseColdLine(out []byte, id string) (int64, bool) {
	prefix := "COLD " + id + " "
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		rest, ok := strings.CutPrefix(line, prefix)
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
		if err != nil {
			continue
		}
		return n, true
	}
	return 0, false
}
