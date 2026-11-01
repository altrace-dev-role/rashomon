//go:build perf

package perf

import (
	"bufio"
	"bytes"
	"os/exec"
	"regexp"
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
func coldCallAllocOnce(t *testing.T, id string) int64 {
	t.Helper()
	pattern := "TestColdCall/" + regexp.QuoteMeta(id)
	cmd := exec.Command("go", "test", "-run", pattern, "-v", "-count=1", "./cmd/rashomon")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -run %s ./cmd/rashomon: %v\n%s", pattern, err, out)
	}
	n, found := parseColdLine(out, id)
	if !found {
		t.Fatalf("go test -run %s ./cmd/rashomon printed no %q line:\n%s", pattern, "COLD "+id, out)
	}
	return n
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
