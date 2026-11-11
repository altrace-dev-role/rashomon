package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestK7_HookPathLatency measures the hook path over a ~50K-record index:
// load from disk, then Scope, Traverse, Rank, Select. RASHOMON_K7=1 runs
// it; it prints p50 and p95 over RASHOMON_K7_RUNS runs (default 30). The
// requirement is at most 300 ms at UserPromptSubmit (kill over 1500 ms).
func TestK7_HookPathLatency(t *testing.T) {
	if os.Getenv("RASHOMON_K7") == "" {
		t.Skip("RASHOMON_K7 not set")
	}
	runs := 30
	if v, err := strconv.Atoi(os.Getenv("RASHOMON_K7_RUNS")); err == nil && v > 0 {
		runs = v
	}
	records := 50000
	if v, err := strconv.Atoi(os.Getenv("RASHOMON_K7_RECORDS")); err == nil && v > 0 {
		records = v
	}
	dir := t.TempDir()
	s := graphtest.Synthetic(records)
	rev := graph.RevCommit("C1")
	if err := graph.SaveFile(dir, s, graph.Header{Repo: "k7", Rev: graph.HeaderRev{Commit: "C1"}}); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(filepath.Join(dir, filepath.FromSlash(graph.GraphFile)))
	t.Logf("records: %d nodes, %d edges; file %d bytes", len(s.Nodes(rev)), len(s.Edges(rev)), st.Size())
	prompt := "make session expiry respect the org-level timeout override."
	var load, build, total []time.Duration
	var last Brief
	for i := 0; i < runs; i++ {
		t0 := time.Now()
		g, h, err := graph.LoadFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		t1 := time.Now()
		last = Build(g, graph.RevCommit(h.Rev.Commit), prompt, Options{IDF: true})
		t2 := time.Now()
		load, build, total = append(load, t1.Sub(t0)), append(build, t2.Sub(t1)), append(total, t2.Sub(t0))
	}
	t.Logf("runs %d: load p50 %s p95 %s; pipeline p50 %s p95 %s; total p50 %s p95 %s",
		runs, pct(load, 50), pct(load, 95), pct(build, 50), pct(build, 95), pct(total, 50), pct(total, 95))
	t.Logf("seeds %d, reached %d, lines %d", len(last.Seeds), last.Neighborhood, len(last.Lines))
	fmt.Fprintf(os.Stderr, "K7 total p95 %s (seeds %d reached %d)\n", pct(total, 95), len(last.Seeds), last.Neighborhood)
}

func pct(ds []time.Duration, p int) time.Duration {
	c := append([]time.Duration(nil), ds...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	i := (len(c)*p + 99) / 100
	if i >= len(c) {
		i = len(c) - 1
	}
	return c[i].Round(100 * time.Microsecond)
}
