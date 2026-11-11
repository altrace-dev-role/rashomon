package index_test

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// AC-M1.0 (PLAN section 8), the rashomon-level equalities: an index built
// over this repository's own pgraph sync carries the sync whole. A unit
// node per package directory the sync holds; a sym node per symbol the
// symbols file places or the facts name within a synced package, and no
// other; a PROVEN Calls edge per distinct calls row whose two ends are such
// symbols, and no other; the header's schema and repository and the repo:
// node. The expected sets are computed from platform.Snapshot alone, by the
// sidecar's own spelling (kind:<import path>.<qualified>), and the actual
// sets are read back from the store's ids and attrs, so a symbol dropped,
// duplicated or misfiled by the resolver, or an edge lost between the log
// and the store, is named by its pgraph id.
//
// Reads the worktree's own sidecars, so it runs under RASHOMON_ACM10=1 and
// skips otherwise (the fixture-level counts are TestGopg_RecordsForFixture);
// RASHOMON_ACM10_DIR names another synced, configured root. The sync must
// be current: run `pgraph sync <root>` first, or a stale symbols file
// refuses the read (ErrUnvouched) and the test fails, by design.
func TestACM10_RashomonIndexCarriesTheSyncWhole(t *testing.T) {
	if os.Getenv("RASHOMON_ACM10") == "" {
		t.Skip("RASHOMON_ACM10 not set")
	}
	root := os.Getenv("RASHOMON_ACM10_DIR")
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		root = filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	}
	for _, need := range []string{"go.mod", ".rashomon/graph.json", ".pgraph/pgraph.walk"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(need))); err != nil {
			t.Fatalf("%s has no %s: index and sync the root first (%v)", root, need, err)
		}
	}
	snap, err := platform.ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatalf("ReadSnapshot(%s): %v", root, err)
	}
	cfg, err := index.ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	const commit = "acm10"
	res, err := index.Build(root, commit, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit(commit)
	if res.Header.Schema != graph.Schema || res.Header.Repo == "" {
		t.Errorf("header = %+v; want schema %d and a repository id", res.Header, graph.Schema)
	}
	if _, ok := res.Store.Node(graph.RepoNodeID(res.Header.Repo), rev); !ok {
		t.Errorf("no repo: node for %q", res.Header.Repo)
	}

	// The sync's universe: each package's directory, and membership of a
	// pgraph id by the place's package or, unplaced, the longest synced
	// import path that prefixes it.
	dirs := map[string]string{} // import path -> directory
	var byLen []string
	for pkg, fs := range snap.Files {
		if len(fs) > 0 {
			dirs[pkg] = path.Dir(fs[0])
			byLen = append(byLen, pkg)
		}
	}
	sort.Slice(byLen, func(i, j int) bool { return len(byLen[i]) > len(byLen[j]) })
	placed := map[string]string{} // id -> package
	for _, p := range snap.Places {
		placed[p.ID] = p.Pkg
	}
	member := func(id string) (string, bool) {
		kind, rest, ok := strings.Cut(id, ":")
		if !ok || kind == "pkg" {
			return "", false
		}
		pkg, ok := placed[id]
		if !ok {
			for _, cand := range byLen {
				if strings.HasPrefix(rest, cand+".") {
					pkg, ok = cand, true
					break
				}
			}
		}
		if _, synced := dirs[pkg]; !ok || !synced || !strings.HasPrefix(rest, pkg+".") {
			return "", false
		}
		return pkg, true
	}
	wantUnits := map[string]bool{}
	for _, d := range dirs {
		wantUnits[d] = true
	}
	wantSyms := map[string]bool{}
	for _, p := range snap.Places {
		if _, ok := member(p.ID); ok {
			wantSyms[p.ID] = true
		}
	}
	wantCalls := map[[2]string]bool{}
	for _, f := range snap.Facts {
		for _, id := range [2]string{f.Subject, f.Object} {
			if _, ok := member(id); ok {
				wantSyms[id] = true
			}
		}
		if f.Predicate != platform.PredCalls {
			continue
		}
		if _, ok := member(f.Subject); !ok {
			continue
		}
		if _, ok := member(f.Object); !ok {
			continue
		}
		wantCalls[[2]string{f.Subject, f.Object}] = true
	}

	// The store, read back in the sidecar's spelling.
	gotUnits := map[string]bool{}
	gotSyms := map[string]bool{}
	pgraphID := map[graph.ID]string{}
	for _, n := range res.Store.Nodes(rev) {
		switch n.Kind {
		case "unit":
			if n.Attrs["pkg"] != "" { // the resolver's units; a declared unit with no package behind it carries no pkg
				gotUnits[n.Attrs["path"]] = true
			}
		case "sym":
			parts := strings.SplitN(string(n.ID), ":", 5) // sym:<lang>:<unit>:<symkind>:<qualified>
			if len(parts) != 5 || parts[1] != "go" {
				t.Errorf("sym node %s: not a Go symbol id", n.ID)
				continue
			}
			id := parts[3] + ":" + n.Attrs["pkg"] + "." + parts[4]
			if gotSyms[id] {
				t.Errorf("sym %s appears twice", id)
			}
			gotSyms[id] = true
			pgraphID[n.ID] = id
			if n.Tier != graph.Proven {
				t.Errorf("sym %s: tier %s; want PROVEN", id, n.Tier)
			}
		}
	}
	gotCalls := map[[2]string]bool{}
	for _, e := range res.Store.Edges(rev) {
		if e.Pred != graph.Calls {
			continue
		}
		k := [2]string{pgraphID[e.Src], pgraphID[e.Dst]}
		if gotCalls[k] {
			t.Errorf("Calls %s -> %s appears twice", k[0], k[1])
		}
		gotCalls[k] = true
		if e.Tier != graph.Proven {
			t.Errorf("Calls %s -> %s: tier %s; want PROVEN", k[0], k[1], e.Tier)
		}
	}

	diff := func(name string, want, got map[string]bool) {
		var missing, extra []string
		for k := range want {
			if !got[k] {
				missing = append(missing, k)
			}
		}
		for k := range got {
			if !want[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		if len(missing)+len(extra) > 0 {
			t.Errorf("%s: want %d, got %d; missing %d %s; extra %d %s", name, len(want), len(got), len(missing), first(missing), len(extra), first(extra))
		}
	}
	diff("units", wantUnits, gotUnits)
	diff("syms", wantSyms, gotSyms)
	wc, gc := map[string]bool{}, map[string]bool{}
	for k := range wantCalls {
		wc[k[0]+" -> "+k[1]] = true
	}
	for k := range gotCalls {
		gc[k[0]+" -> "+k[1]] = true
	}
	diff("Calls", wc, gc)
	t.Logf("root %s: %d packages, %d symbols, %d calls rows; store %d files", root, len(dirs), len(wantSyms), len(wantCalls), res.Files)
}

// first is at most the first five of a sorted list, for a failure message.
func first(xs []string) []string {
	if len(xs) > 5 {
		return xs[:5]
	}
	return xs
}
