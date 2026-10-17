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
// the RSS rows, this is not a per-platform figure yet, because it has not
// been measured on a second platform.
const coldToleranceFraction = 0.10

// TestColdAllocBudget gates budgets.json's cold_alloc_bytes_max entries
// against a real measurement: for each entry whose field is nonzero, it
// spawns a fresh `go test -run` process against cmd/rashomon's TestColdCall
// (one process per entry, so the figure is a genuinely cold one, not a
// second subtest sharing the first's warmed process state) and reads the one
// "COLD <id> <bytes>" line the subtest prints to real stdout.
//
// Entries whose cold_alloc_bytes_max is 0 (recap.*, probe.*, and anything
// with no benchmark row) are skipped: that 0 means "not yet measured" per
// the field's own doc comment, not a budget of zero.
//
// -v is required on the child's `go test` invocation: go test buffers a
// passing test's stdout and only ever shows it with -v or on failure, a
// behavior checked here against the assumption in an earlier design note
// that CombinedOutput alone would see it; it does not.
func TestColdAllocBudget(t *testing.T) {
	bud := loadBudgets(t)
	for _, e := range bud.Entries {
		if e.ColdAllocBytesMax <= 0 {
			continue
		}
		e := e
		t.Run(e.ID, func(t *testing.T) {
			measured := coldCallAlloc(t, e.ID)
			if OverBudget(float64(measured), float64(e.ColdAllocBytesMax), coldToleranceFraction) {
				t.Errorf("%s: cold call allocated %d bytes, over budget %d (+%.0f%%)", e.ID, measured, e.ColdAllocBytesMax, coldToleranceFraction*100)
				return
			}
			t.Logf("%s: cold call allocated %d bytes against budget %d (+%.0f%%)", e.ID, measured, e.ColdAllocBytesMax, coldToleranceFraction*100)
		})
	}
}

// coldCallAlloc runs cmd/rashomon's TestColdCall/<id> subtest in its own
// `go test` process and returns the one TotalAlloc delta it printed.
func coldCallAlloc(t *testing.T, id string) int64 {
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
