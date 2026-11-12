package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The K7 conflict measures (W-M2.4 hook seam, card m2-seam1): the post's
// conflict check timed on the real PostToolUse entry over cfProject, one
// fresh session per run, its base written by the real prompt entry; and the
// base's size over this repository's own index. They report, and gate only
// each case's outcome: RASHOMON_K7=1 runs them, RASHOMON_K7_RUNS is the run
// count (k7UnitBriefRuns). The post's target is p95 at or under 150 ms; the
// base's cap is maxConflictBaseBytes.

// k7ConflictOutcome is the session's last conflict check as a word: "none"
// when it has none, else the check's row count.
func k7ConflictOutcome(s *planSession) string {
	cs := s.run().ConflictChecks
	if len(cs) == 0 {
		return "none"
	}
	return fmt.Sprintf("rows:%d", len(cs[len(cs)-1].Rows))
}

// TestK7_PostPathConflicts times the post of a Write: a new crossing of b1
// with the base present, a file in b1's scope that crosses nothing, a file
// under no rule, and the crossing with no base (no prompt ran), whose post
// finds no base file and checks nothing.
func TestK7_PostPathConflicts(t *testing.T) {
	runs := k7UnitBriefRuns(t)
	for _, c := range []struct {
		name, rel, body string
		prompt          bool
		want            string
	}{
		{"crossing, base present", cfGraph, cfCross, true, "rows:1"},
		{"in scope, no crossing", cfGraph, "package graph\n\nfunc K() {}\n", true, "rows:0"},
		{"under no rule", cfHookio, "package hookio\n\nfunc K() {}\n", true, "none"},
		{"crossing, no base", cfGraph, cfCross, false, "none"},
	} {
		var ds []time.Duration
		var allocs []uint64
		outcomes := map[string]int{}
		for i := 0; i < runs; i++ {
			s := cfSession(t, store.RecordScopeV5, true)
			if c.prompt {
				s.prompt("fix " + cfGraph)
			}
			d, a := k7Measure(func() { s.land("toolu_k7_cf", c.rel, c.body) })
			outcomes[k7ConflictOutcome(s)]++
			ds, allocs = append(ds, d), append(allocs, a)
		}
		if outcomes[c.want] != runs {
			t.Errorf("%s: outcomes %v, want %s in every run", c.name, outcomes, c.want)
		}
		k7Report(t, c.name, ds, allocs, fmt.Sprintf("outcomes %v", outcomes))
	}
}

// TestK7_ConflictBaseOnTheRepoIndex sizes the base over this repository's
// own index (D2's cap). The slice carries, for boundary rules, each edge of
// a crossing pred whose ends lie under a rule's From and To (with their
// Contains edges and a sym source's declarers), and for path rules the
// digest of each file they reach. So a boundary rule from every Go unit to
// every Go unit carries every such edge, and a protect_path rule applying
// to every Go unit a digest per Go file: together the most a base over
// this index holds. A rule from every Go unit into internal/graph alone is
// the broad rule a spec might declare. Each size is reported against
// maxConflictBaseBytes, not gated: a base over the cap is not written and
// that session's posts check nothing (a debt row).
func TestK7_ConflictBaseOnTheRepoIndex(t *testing.T) {
	if os.Getenv("RASHOMON_K7") == "" {
		t.Skip("RASHOMON_K7 not set")
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if graph.FindIndex(repo) != repo {
		t.Skip("no index at the module root: run rashomon graph index .")
	}
	g, h, err := graph.LoadFile(repo)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit(h.Rev.Commit)
	files := map[graph.ID]int{}
	for _, n := range g.Nodes(rev) {
		if n.Kind == "unit" && strings.HasPrefix(string(n.ID), "unit:go:") {
			files[n.ID] = len(g.Out(n.ID, rev, graph.Contains))
		}
	}
	units := slices.Sorted(maps.Keys(files))
	if len(units) == 0 {
		t.Fatal("the index holds no Go unit")
	}
	total := 0
	var every []string
	for _, u := range units {
		every = append(every, string(u))
		total += files[u]
	}
	t.Logf("index at %s: %d Go units, %d files under them", h.Rev.Commit, len(units), total)
	all := engine.Rule{ID: "k7a", Kind: "boundary", From: every, To: every}
	into := engine.Rule{ID: "k7g", Kind: "boundary", From: every, To: []string{"unit:go:internal/graph"}}
	keep := engine.Rule{ID: "k7p", Kind: "protect_path", AppliesTo: units}
	head := conflictBaseHead{V: conflictBaseVersion, Commit: h.Rev.Commit, Repo: h.Repo, RulesDigest: strings.Repeat("0", 32)}
	for _, c := range []struct {
		name  string
		rules []engine.Rule
	}{
		{"boundary, every Go unit to every Go unit, and protect_path over every Go unit (the most)", []engine.Rule{all, keep}},
		{"boundary, every Go unit to every Go unit", []engine.Rule{all}},
		{"protect_path over every Go unit", []engine.Rule{keep}},
		{"boundary, every Go unit into internal/graph", []engine.Rule{into}},
	} {
		cb := engine.ConflictBaseOf(g, rev, c.rules)
		data, err := encodeConflictBase(head, c.rules, cb)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: base %d B of the %d B cap (%.0f%%), %d records, %d digests",
			c.name, len(data), maxConflictBaseBytes, 100*float64(len(data))/maxConflictBaseBytes, len(cb.Records), len(cb.Digests))
	}
}
