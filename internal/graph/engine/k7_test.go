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
)

// synthetic builds a graph of about `records` records shaped like a
// repository: files declaring functions and constants, calls between
// functions, table reads and writes, lessons on tables. Names come from a
// vocabulary so a prompt's words hit a realistic fraction.
func synthetic(records int) *graph.Store {
	vocab := []string{"session", "org", "user", "invoice", "token", "cache", "expire", "timeout", "login", "plan", "seat", "key", "report", "purge", "settings", "notify", "email", "queue", "audit", "limit"}
	rev := graph.RevCommit("C1")
	s := graph.NewStore()
	prov := graph.Prov{Extractor: "synthetic", Version: "1"}
	perFile := 12
	files := records / (perFile*3 + 2)
	var syms []graph.ID
	for f := 0; f < files; f++ {
		unit := "services/" + vocab[f%len(vocab)]
		mod := vocab[f%len(vocab)] + "." + vocab[(f/len(vocab))%len(vocab)] + strconv.Itoa(f)
		path := unit + "/" + vocab[(f/len(vocab))%len(vocab)] + strconv.Itoa(f) + ".py"
		file := graph.FileID(path)
		s.AssertNode(graph.Node{ID: file, Kind: "file", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov, Attrs: map[string]string{"module": mod}})
		for i := 0; i < perFile; i++ {
			name := vocab[(f+i)%len(vocab)] + "_" + vocab[(f*7+i*3)%len(vocab)] + strconv.Itoa(i)
			id := graph.SymID("py", unit, "func", mod+"."+name)
			s.AssertNode(graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov,
				Attrs: map[string]string{"symkind": "func", "sig": "(" + vocab[(i*5+f)%len(vocab)] + "_id, " + vocab[(i*11+f)%len(vocab)] + ")", "line": strconv.Itoa(i * 9)}})
			s.AssertEdge(graph.Edge{Src: file, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
			if len(syms) > 0 {
				s.AssertEdge(graph.Edge{Src: id, Pred: graph.Calls, Dst: syms[(f*perFile+i*31)%len(syms)], Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: prov})
			}
			if i%4 == 0 {
				tbl := graph.TableID("public." + vocab[(f+i)%len(vocab)] + "s")
				s.AssertNode(graph.Node{ID: tbl, Kind: "table", Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: prov})
				pred := graph.Reads
				if i%8 == 0 {
					pred = graph.Writes
				}
				s.AssertEdge(graph.Edge{Src: id, Pred: pred, Dst: tbl, Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: prov, Attrs: map[string]string{"cols": "org_id,expires_at"}})
			}
			syms = append(syms, id)
		}
		cid := graph.SymID("py", unit, "const", mod+"."+vocab[f%len(vocab)]+"_TTL")
		s.AssertNode(graph.Node{ID: cid, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov, Attrs: map[string]string{"symkind": "const", "value": "3600"}})
		s.AssertEdge(graph.Edge{Src: file, Pred: graph.Declares, Dst: cid, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	}
	for i := 0; i < len(vocab); i++ {
		les := graph.ID("les:INC-" + strconv.Itoa(100+i))
		s.AssertNode(graph.Node{ID: les, Kind: "lesson", Tier: graph.Declared, Conf: 1, Rev: rev, Attrs: map[string]string{"title": "incident on " + vocab[i], "text": "INC: the " + vocab[i] + " table is purged nightly; change its predicate with any lifetime change."}})
		s.AssertEdge(graph.Edge{Src: les, Pred: graph.AppliesTo, Dst: graph.TableID("public." + vocab[i] + "s"), Tier: graph.Declared, Conf: 1, Rev: rev})
	}
	return s
}

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
	s := synthetic(records)
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
