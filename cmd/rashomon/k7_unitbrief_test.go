package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The K7 unit-brief measures (W-M2.3 hook half): the post's unit-brief join
// timed on the real PostToolUse entry over planHookProject, one fresh
// session per run, and the prompt side's unit-brief writes timed on
// graphBrief over a 50000-record index whose files sit in units. They
// report and gate nothing: RASHOMON_K7=1 runs them; RASHOMON_K7_RUNS is the
// run count (default 30). Each run is timed in process after a collection,
// its allocation count the mallocs it made; the post's target is p95 at or
// under 150 ms.

// k7PadLines is the steer log's padding, in lines, of the cases that ask
// what the join's steer-log read costs in a long session.
const k7PadLines = 2000

// k7UnitBriefRuns is RASHOMON_K7_RUNS, default 30; the test is skipped
// unless RASHOMON_K7 is set.
func k7UnitBriefRuns(t *testing.T) int {
	t.Helper()
	if os.Getenv("RASHOMON_K7") == "" {
		t.Skip("RASHOMON_K7 not set")
	}
	if v, err := strconv.Atoi(os.Getenv("RASHOMON_K7_RUNS")); err == nil && v > 0 {
		return v
	}
	return 30
}

// k7Measure is f's wall time and the mallocs it made, both read outside
// the timed span, after a collection, so one run's garbage is not the
// next's.
func k7Measure(f func()) (time.Duration, uint64) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	d := time.Since(start)
	runtime.ReadMemStats(&after)
	return d, after.Mallocs - before.Mallocs
}

// k7Median is the median of n.
func k7Median(n []uint64) uint64 {
	c := slices.Clone(n)
	slices.Sort(c)
	return c[len(c)/2]
}

// k7Report logs one case's p50 and p95 and its median mallocs.
func k7Report(t *testing.T, name string, ds []time.Duration, allocs []uint64, note string) {
	t.Helper()
	t.Logf("%s: p50 %v p95 %v, median mallocs %d over %d runs; %s",
		name, percentile(ds, 50), percentile(ds, 95), k7Median(allocs), len(ds), note)
}

// steerPath is the session's steer log.
func (s *planSession) steerPath() string {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	return filepath.Join(st.RunDir(benchSessionID), store.FileSteer)
}

// steerBytes is the session's steer log's size, 0 when it has none.
func (s *planSession) steerBytes() int64 {
	s.t.Helper()
	fi, err := os.Stat(s.steerPath())
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return fi.Size()
}

// padSteer appends n lines to the session's steer log as earlier prompts
// left them: each the line padLine under the blast-radius pattern and a key
// of its own, so no unit-brief read counts one given, ten lines to a prompt
// id of its own, each a millisecond after the last and all before padLine.
// The prompt budget's tail scan (steer countBudget) stops at the first line
// of another prompt or, at the tail, of another append; lines of the
// current prompt, or one append's worth at the tail, it reads through, and
// a log padded so measures the budget scan, not the join's read. Pad before
// a session's current prompt writes its lines.
func (s *planSession) padSteer(padLine []byte, n int) {
	s.t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(padLine, &m); err != nil {
		s.t.Fatal(err)
	}
	at, err := strconv.ParseInt(string(m["at_unix_ms"]), 10, 64)
	if err != nil {
		s.t.Fatalf("pad line's at_unix_ms: %v", err)
	}
	m["pattern"] = json.RawMessage(strconv.Quote(string(pattern.BlastRadius)))
	var buf bytes.Buffer
	for i := 0; i < n; i++ {
		m["key"] = json.RawMessage(strconv.Quote("pad:" + strconv.Itoa(i)))
		m["prompt_id"] = json.RawMessage(strconv.Quote("k7-pad-" + strconv.Itoa(i/10)))
		m["at_unix_ms"] = json.RawMessage(strconv.FormatInt(at-int64(n-i), 10))
		b, err := json.Marshal(m)
		if err != nil {
			s.t.Fatal(err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(s.steerPath(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		s.t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		s.t.Fatal(err)
	}
}

// k7Outcome is what the measured edit left, against the brief lines and
// the misses the session held before it: miss:<reason> for each new miss,
// delivered for a new brief line, none for neither.
func (s *planSession) k7Outcome(lines, misses int) string {
	s.t.Helper()
	var out []string
	for _, m := range s.misses()[misses:] {
		out = append(out, "miss:"+m.Reason)
	}
	if len(s.briefLines()) > lines {
		out = append(out, "delivered")
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, "+")
}

// k7PadLine is the raw steer line the fixture brief's delivery writes, in
// a session of its own: the padding's model line.
func k7PadLine(t *testing.T) []byte {
	t.Helper()
	s := ubPostSession(t, store.RecordScopeV5)
	s.writeBrief("c1")
	s.editFile("toolu_k7_pad", planHookHeld)
	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := st.ReadSteer(benchSessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range raw {
		var l struct{ Key string }
		if json.Unmarshal(b, &l) == nil && strings.HasPrefix(l.Key, unitBriefKeyPrefix) {
			return b
		}
	}
	t.Fatal("the fixture brief's delivery wrote no unit-brief line")
	return nil
}

// TestK7_PostPathUnitBrief times the post on an edit in the fixture's unit:
// the first edit with the widest brief the prompt side stores (the larger
// file of maxBriefShapes' widest five-line briefs), with the fixture's
// brief, and with none (an absent miss); then an edit after the unit's
// brief was given, with the steer log padded by k7PadLines lines of earlier
// prompts and without, and an absent miss under the same padding, whose
// join reads no steer log (its miss is counted at the file's Lstat), so any
// difference it shows from the unpadded absent miss is another reader's.
// Every edit of the fixture's file raises the spec's protect_path finding,
// so every measured post routes, and its budget scan reads the log's tail.
func TestK7_PostPathUnitBrief(t *testing.T) {
	runs := k7UnitBriefRuns(t)
	own := graph.GlobalID(planRID, ubPostUnit)
	var widest []engine.Line
	size := 0
	for name, shape := range maxBriefShapes() {
		b, w := widestBrief(shape)
		if b.Kept != 5 {
			t.Fatalf("%s: deliver keeps no five-line brief", name)
		}
		lines := shape(w)
		data, err := encodeUnitBrief(engine.Rebind(graph.RevCommit("c1"), lines), own, graph.GlobalID(planRID, b.FirstKey()), graph.Algorithmic, "prompt-1")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%s: widest brief at width %d, file %d B", name, w, len(data))
		if len(data) > size {
			widest, size = lines, len(data)
		}
	}
	padLine := k7PadLine(t)
	t.Logf("pad line %d B, %d lines", len(padLine), k7PadLines)
	give := func(s *planSession) {
		s.writeBrief("c1")
		s.editFile("toolu_k7_give", planHookHeld)
		if n := len(s.briefLines()); n != 1 {
			t.Fatalf("the giving edit left %d brief lines, want 1", n)
		}
	}
	absent := "miss:" + store.UnitBriefAbsent
	for _, c := range []struct {
		name  string
		setup func(s *planSession)
		want  string // the measured edit's outcome; "" reports it only
	}{
		{"widest brief, first edit", func(s *planSession) { s.storeLines(ubPostUnit, own, "c1", widest) }, ""},
		{"fixture brief, first edit", func(s *planSession) { s.writeBrief("c1") }, "delivered"},
		{"no brief", func(*planSession) {}, absent},
		{"given, steer log padded", func(s *planSession) { s.padSteer(padLine, k7PadLines); give(s) }, "none"},
		{"no brief, steer log padded", func(s *planSession) { s.padSteer(padLine, k7PadLines) }, absent},
		{"given, steer log unpadded", give, "none"},
	} {
		var ds []time.Duration
		var allocs []uint64
		outcomes := map[string]int{}
		var steer int64
		for i := 0; i < runs; i++ {
			s := ubPostSession(t, store.RecordScopeV5)
			c.setup(s)
			steer = s.steerBytes()
			lines, misses := len(s.briefLines()), len(s.misses())
			d, a := k7Measure(func() { s.editFile("toolu_k7_edit", planHookHeld) })
			outcomes[s.k7Outcome(lines, misses)]++
			ds, allocs = append(ds, d), append(allocs, a)
		}
		if c.want != "" && outcomes[c.want] != runs {
			t.Errorf("%s: outcomes %v, want %s in every run", c.name, outcomes, c.want)
		}
		k7Report(t, c.name, ds, allocs, fmt.Sprintf("steer log %d B before the edit, outcomes %v", steer, outcomes))
	}
}

// k7UnitGraph is graphtest.Synthetic(records) with each file contained by
// the unit of its directory, as pylite asserts it (unit:py:<dir> contains
// file): Synthetic asserts no unit, so over it no prompt names one and
// writeUnitBriefs stores nothing.
func k7UnitGraph(records int) *graph.Store {
	s := graphtest.Synthetic(records)
	rev := graph.RevCommit(graphtest.Commit)
	prov := graph.Prov{Extractor: "pylite", Version: "1"}
	seen := map[graph.ID]bool{}
	for _, n := range s.Nodes(rev) {
		if n.Kind != "file" {
			continue
		}
		dir := path.Dir(strings.TrimPrefix(string(n.ID), "file:"))
		u := graph.UnitID("py", dir)
		if !seen[u] {
			seen[u] = true
			s.AssertNode(graph.Node{ID: u, Kind: "unit", Tier: graph.Declared, Conf: 1, Rev: rev,
				Attrs: map[string]string{"path": dir, "lang": "py"}})
		}
		s.AssertEdge(graph.Edge{Src: u, Pred: graph.Contains, Dst: n.ID, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	}
	return s
}

// k7Stored is the number of files under dir and their bytes, none when dir
// is absent.
func k7Stored(t *testing.T, dir string) (int, int64) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0
	}
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		sum += fi.Size()
	}
	return len(ents), sum
}

// TestK7_PromptUnitBriefs times the prompt side's graph read (graphBrief)
// over a 50000-record index whose files sit in units, with the
// session-content marker and without: the difference is writeUnitBriefs,
// the unit briefs' build and writes, which run only with the marker (the
// evidence file is written either way). Each run is a session of its own,
// so no run replaces another's files. A prompt that says no brief is
// reported with the engine's last decision; marker runs that store no unit
// brief for any prompt are a failure, the difference measuring nothing.
func TestK7_PromptUnitBriefs(t *testing.T) {
	runs := k7UnitBriefRuns(t)
	dir := t.TempDir()
	if err := graph.SaveFile(dir, k7UnitGraph(50000), graph.Header{Repo: "k7", Rev: graph.HeaderRev{Commit: graphtest.Commit}}); err != nil {
		t.Fatal(err)
	}
	most := 0
	for _, prompt := range []string{
		"make session expiry respect the org-level timeout override.",
		"audit the invoice seat queue email limits",
		"fix the invoice email queue: the seat limit check and the audit report both drop rows for the org",
		"the nightly purge of expired tokens in the cache misses the notify email queue and the login settings",
	} {
		for _, marker := range []bool{false, true} {
			root := t.TempDir()
			if marker {
				if err := os.WriteFile(filepath.Join(root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var ds []time.Duration
			var allocs []uint64
			files := map[int]int{}
			var stored int64
			var ib indexBrief
			said := 0
			for i := 0; i < runs; i++ {
				sid := "k7-" + strconv.Itoa(i)
				ok := false
				d, a := k7Measure(func() { ib, ok = graphBrief(root, dir, sid, "p1", []byte(prompt)) })
				if ok {
					said++
				}
				n, b := k7Stored(t, graph.UnitBriefDir(root, sid))
				files[n]++
				stored = b
				most = max(most, n)
				ds, allocs = append(ds, d), append(allocs, a)
			}
			if !marker && files[0] != runs {
				t.Errorf("%q: runs without the marker stored unit briefs: %v", prompt, files)
			}
			why := "a brief in every run"
			if said < runs {
				last := "none"
				if n := len(ib.Decisions); n > 0 {
					last = ib.Decisions[n-1].Why
				}
				why = fmt.Sprintf("a brief in %d runs; the last decision %q", said, last)
			}
			k7Report(t, fmt.Sprintf("%q, marker %v", prompt, marker), ds, allocs,
				fmt.Sprintf("%s; %d brief lines, unit-brief files per run %v, the last run's %d B", why, len(ib.Lines), files, stored))
		}
	}
	if most == 0 {
		t.Error("no marker run stored a unit brief: the marker's difference measures nothing")
	}
}
