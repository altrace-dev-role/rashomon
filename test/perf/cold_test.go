//go:build perf

package perf

import (
	"bufio"
	"bytes"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// coldToleranceFraction is the margin ColdAllocBytesMax is gated at, the same
// flat ten percent AllocBytesMax's benchmark rows use (TestRES36f_); unlike
// the RSS rows, this is not a per-platform figure yet, because it has not
// been measured on a second platform.
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
	for _, e := range bud.Entries {
		if e.ColdAllocBytesMax <= 0 {
			continue
		}
		e := e
		t.Run(e.ID, func(t *testing.T) {
			ran[ColdGate(e.ID)] = true
			measured, all := coldCallAlloc(t, e.ID)
			if OverBudget(float64(measured), float64(e.ColdAllocBytesMax), coldToleranceFraction) {
				t.Errorf("%s: cold call allocated %d bytes (runs %v), over budget %d (+%.0f%%)", e.ID, measured, all, e.ColdAllocBytesMax, coldToleranceFraction*100)
				return
			}
			t.Logf("%s: cold call allocated %d bytes (runs %v) against budget %d (+%.0f%%)", e.ID, measured, all, e.ColdAllocBytesMax, coldToleranceFraction*100)
		})
	}
}

// coldLevelRuns is how many fresh processes TestPE1_ColdCallIsOneLevel
// spawns, and coldLevelSpreadBytes how far apart their figures may sit. At
// one P, twenty fresh post.bash_short calls spread 1440 B (152640 to 154080,
// pe1-gmp); the two levels an unpinned process draws sit 3.7K or more apart.
const (
	coldLevelRuns        = 10
	coldLevelSpreadBytes = 2048
)

// TestPE1_ColdCallIsOneLevel (card PERF-EDGE1): the cold figure is one
// level, not two. A fresh process's post call either found json/v2's pooled
// decoder on the P it ran on or missed it and paid a new decoder's string
// cache, so at GOMAXPROCS=3 eleven of twenty runs read 153.3K to 154.7K and
// nine read 158.4K to 161.5K (pe1-gmp; CI macos read [163256 156312
// 156152]), and TestColdAllocBudget's minimum of three went red whenever all
// three drew the high level. TestColdCall pins one P for its subtest and
// coldCallAllocOnce starts its process at one P, so ten fresh runs sit
// within coldLevelSpreadBytes. Break: both pins dropped, and ten runs draw
// both levels; the subtest's pin alone still drew one high run in ten at
// eight P (pe1-cold2: 160784 beside nine at 155144 to 155976).
func TestPE1_ColdCallIsOneLevel(t *testing.T) {
	const id = "post.bash_short"
	all := make([]int64, coldLevelRuns)
	for i := range all {
		all[i] = coldCallAllocOnce(t, id)
	}
	lo, hi := slices.Min(all), slices.Max(all)
	if hi-lo > coldLevelSpreadBytes {
		t.Errorf("%s: %d fresh cold calls spread %d bytes (%d to %d, runs %v), want within %d: the cold figure has more than one level", id, len(all), hi-lo, lo, hi, all, coldLevelSpreadBytes)
		return
	}
	t.Logf("%s: %d fresh cold calls within %d bytes (%d to %d, runs %v)", id, len(all), hi-lo, lo, hi, all)
}

// coldCallAlloc runs cmd/rashomon's TestColdCall/<id> subtest in coldRuns
// fresh `go test` processes and returns the minimum of the TotalAlloc
// deltas each one printed, and every run's figure for the failure message.
func coldCallAlloc(t *testing.T, id string) (int64, []int64) {
	t.Helper()
	all := make([]int64, coldRuns)
	for i := range all {
		all[i] = coldCallAllocOnce(t, id)
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
// own `go test` process and returns the one TotalAlloc delta it printed.
// The process starts at one P (PERF-EDGE1): TestColdCall's pin holds from
// its subtest's first line, but the runtime and the testing package start at
// every P the machine has, and a start at one P, GOMAXPROCS=1 in the
// environment, drew no high run in thirty (pe1-gmp, pe1-cold2). -exec starts
// the test binary through env, so the go command still builds at the
// machine's P.
func coldCallAllocOnce(t *testing.T, id string) int64 {
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
	t.Logf("cold %s: %d bytes; %s", id, n, coldGCLine(out, id))
	return n
}

// coldGCLine returns the "COLDGC <id> ..." line TestColdCall prints for id
// beside its COLD line (PERF-EDGE1 probe), so each figure is logged with the
// GC cycles that began around it.
func coldGCLine(out []byte, id string) string {
	prefix := "COLDGC " + id + " "
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return "no COLDGC line"
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
