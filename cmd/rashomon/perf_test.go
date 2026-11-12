package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestK7_PromptPathOnRashomon (W-M1.8, AC-M1.2) measures the hook's own
// prompt path over this repository's index: graphBrief, which finds the
// index above cwd, loads it, builds the brief for the T1 prompt, writes
// the evidence file and names the first line's node and unit, p50 and p95
// over RASHOMON_K7_RUNS runs (default 30). RASHOMON_K7=1 runs it; the
// store root is a temporary directory, so nothing lands under the real
// store; RASHOMON_K7_DIR names another index. A p95 above 300 ms fails
// (PRD 6.8), and every run must render a brief whose finding checks.
func TestK7_PromptPathOnRashomon(t *testing.T) {
	if os.Getenv("RASHOMON_K7") == "" {
		t.Skip("RASHOMON_K7 not set")
	}
	up := filepath.Join("..", "..")
	cwd := os.Getenv("RASHOMON_K7_DIR")
	if cwd == "" {
		root, err := filepath.Abs(up)
		if err != nil {
			t.Fatal(err)
		}
		cwd = graph.FindIndex(root)
	}
	if cwd == "" {
		t.Skip("no index at or above the module root: run rashomon graph index .")
	}
	prompt := []byte(os.Getenv("RASHOMON_K7_PROMPT"))
	if len(prompt) == 0 {
		b, err := os.ReadFile(filepath.Join(up, "test", "live", "autosteer", "t1", "prompt.txt"))
		if err != nil {
			t.Fatal(err)
		}
		prompt = b
	}
	runs := 30
	if v, err := strconv.Atoi(os.Getenv("RASHOMON_K7_RUNS")); err == nil && v > 0 {
		runs = v
	}
	storeRoot := t.TempDir()
	var total []time.Duration
	var last indexBrief
	for i := 0; i < runs; i++ {
		t0 := time.Now()
		gb, ok := graphBrief(storeRoot, cwd, "k7", strconv.Itoa(i), prompt)
		total = append(total, time.Since(t0))
		if !ok {
			t.Fatalf("run %d rendered no brief", i)
		}
		last = gb
	}
	f, ok := graphFinding(last, keyGraph, 1, nil)
	if !ok {
		t.Fatal("the brief gave no finding")
	}
	if err := f.Check(); err != nil {
		t.Errorf("the brief's finding does not check: %v", err)
	}
	t.Logf("index above %s: repo %s; first %s (%s), unit %s", cwd, last.Repo, last.First, last.Tier, last.Unit)
	t.Logf("runs %d: prompt path p50 %s p95 %s; seeds %d, reached %d, lines %d; brief:\n%s",
		runs, percentile(total, 50), percentile(total, 95), len(last.Seeds), last.Neighborhood, len(last.Lines), last.Text())
	fmt.Fprintf(os.Stderr, "K7 rashomon prompt path p95 %s (graphBrief; lines %d)\n", percentile(total, 95), len(last.Lines))
	if p95 := percentile(total, 95); p95 > 300*time.Millisecond {
		t.Errorf("prompt path p95 %s is over the 300 ms requirement (PRD 6.8, AC-M1.2)", p95)
	}
}

// percentile is the p-th percentile of ds, rounded to a tenth of a millisecond.
func percentile(ds []time.Duration, p int) time.Duration {
	c := append([]time.Duration(nil), ds...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	i := (len(c)*p + 99) / 100
	if i >= len(c) {
		i = len(c) - 1
	}
	return c[i].Round(100 * time.Microsecond)
}
