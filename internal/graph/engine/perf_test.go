package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// k7Index is the index the K7 rows on this repository are measured over:
// RASHOMON_K7_DIR, else the index at or above the module root
// (.rashomon/graph.jsonl, as `rashomon graph index .` writes it); "" when
// there is none, which the caller skips on and says so.
func k7Index(t *testing.T, up string) string {
	t.Helper()
	if dir := os.Getenv("RASHOMON_K7_DIR"); dir != "" {
		return dir
	}
	root, err := filepath.Abs(up)
	if err != nil {
		t.Fatal(err)
	}
	return graph.FindIndex(root)
}

// k7Prompt is the prompt the rows are measured with: RASHOMON_K7_PROMPT,
// else the T1 prompt frozen under test/live/autosteer/t1/prompt.txt (PLAN
// section 7.2), read from the module root so the measurement and the
// experiment share one text.
func k7Prompt(t *testing.T, up string) string {
	t.Helper()
	if p := os.Getenv("RASHOMON_K7_PROMPT"); p != "" {
		return p
	}
	b, err := os.ReadFile(filepath.Join(up, "test", "live", "autosteer", "t1", "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// k7Runs is RASHOMON_K7_RUNS, default 30.
func k7Runs() int {
	if v, err := strconv.Atoi(os.Getenv("RASHOMON_K7_RUNS")); err == nil && v > 0 {
		return v
	}
	return 30
}

// TestK7_RashomonGraph (W-M1.8, AC-M1.2) measures the engine's part of the
// hook path over this repository's own index: load from disk, then Scope,
// Traverse, Rank and Select for the T1 prompt, p50 and p95 over k7Runs
// runs, with the index's header, size and the brief it produced logged.
// RASHOMON_K7=1 runs it. The requirement is a prompt path at or under 300
// ms (PRD 6.8), so a p95 above it fails: a measurement run is a gate, not
// a print.
func TestK7_RashomonGraph(t *testing.T) {
	if os.Getenv("RASHOMON_K7") == "" {
		t.Skip("RASHOMON_K7 not set")
	}
	up := filepath.Join("..", "..", "..")
	dir := k7Index(t, up)
	if dir == "" {
		t.Skip("no index at or above the module root: run rashomon graph index .")
	}
	prompt, runs := k7Prompt(t, up), k7Runs()
	st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(graph.GraphFile)))
	if err != nil {
		t.Fatal(err)
	}
	var load, build, total []time.Duration
	var last Brief
	var h graph.Header
	var nodes, edges int
	for i := 0; i < runs; i++ {
		t0 := time.Now()
		g, hdr, err := graph.LoadFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		t1 := time.Now()
		rev := graph.RevCommit(hdr.Rev.Commit)
		last = Build(g, rev, prompt, Options{IDF: true})
		t2 := time.Now()
		load, build, total = append(load, t1.Sub(t0)), append(build, t2.Sub(t1)), append(total, t2.Sub(t0))
		if i == 0 {
			h, nodes, edges = hdr, len(g.Nodes(rev)), len(g.Edges(rev))
		}
	}
	t.Logf("index %s: repo %s, commit %s, producers %v; %d nodes, %d edges, %d records; file %d bytes",
		dir, h.Repo, h.Rev.Commit, h.Producers, nodes, edges, h.Records, st.Size())
	t.Logf("runs %d: load p50 %s p95 %s; pipeline p50 %s p95 %s; total p50 %s p95 %s",
		runs, pct(load, 50), pct(load, 95), pct(build, 50), pct(build, 95), pct(total, 50), pct(total, 95))
	t.Logf("seeds %d, reached %d, lines %d; brief:\n%s", len(last.Seeds), last.Neighborhood, len(last.Lines), last.Text())
	fmt.Fprintf(os.Stderr, "K7 rashomon total p95 %s (load p95 %s, pipeline p95 %s; seeds %d reached %d lines %d)\n",
		pct(total, 95), pct(load, 95), pct(build, 95), len(last.Seeds), last.Neighborhood, len(last.Lines))
	if p95 := pct(total, 95); p95 > 300*time.Millisecond {
		t.Errorf("prompt path p95 %s is over the 300 ms requirement (PRD 6.8, AC-M1.2)", p95)
	}
}
