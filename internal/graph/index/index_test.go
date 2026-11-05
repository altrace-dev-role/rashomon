package index

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// shapes is the Go producer's fixture (two packages under a pgraph sync)
// copied under a fresh root, with one Python unit beside it.
func shapes(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "r")
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "lang", "gopg", "testdata", "shapes"))); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "py", "app.py"), "def run():\n    return 1\n")
	return root
}

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// twoLangs declares the whole root as Go and py as a Python unit.
var twoLangs = Config{Units: []Unit{
	{Path: ".", Lang: gopg.Lang},
	{Path: "py", ModRoot: "py", Modules: []string{"app"}, Lang: pylite.Lang},
}}

// TestBuild_TwoLanguages (W-M1.4, AC-M1.0): each declared language's
// resolver runs over the files under its units; the store holds the Go
// units pgraph synced at the proven tier, the Python unit as declared, the
// files and symbols of both; the header names the repository, the commit
// and each producer; the repository node carries the id.
func TestBuild_TwoLanguages(t *testing.T) {
	root := shapes(t)
	r, err := Build(root, "c1", twoLangs)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit("c1")
	for id, want := range map[graph.ID]struct {
		kind string
		tier graph.Tier
	}{
		"unit:go:shape":          {"unit", graph.Proven},
		"unit:go:app":            {"unit", graph.Proven},
		"file:shape/shape.go":    {"file", graph.Proven},
		"sym:go:shape:func:Sum":  {"sym", graph.Proven},
		"unit:py:py":             {"unit", graph.Declared},
		"file:py/app.py":         {"file", graph.Algorithmic},
		"sym:py:py:func:app.run": {"sym", graph.Algorithmic},
	} {
		n, ok := r.Store.Node(id, rev)
		if !ok || n.Kind != want.kind || n.Tier != want.tier {
			t.Errorf("%s = %+v, %v; want kind %s at tier %s", id, n, ok, want.kind, want.tier)
		}
	}
	if _, ok := r.Store.Edge("sym:go:app:func:Run", graph.Calls, "sym:go:shape:func:Sum", rev); !ok {
		t.Errorf("the Go call edge is missing")
	}
	if r.Files != 3 {
		t.Errorf("files = %d; want the two Go files and the Python file", r.Files)
	}
	rid, _ := baseline.RepoID(root)
	h := r.Header
	if h.Schema != graph.Schema || h.Repo != rid || h.Rev.Commit != "c1" || !reflect.DeepEqual(h.Producers, map[string]string{"go": "pgraph/v0", "py": "pylite/1"}) {
		t.Errorf("header = %+v; want schema %d, repo %q, commit c1, producers go pgraph/v0 and py pylite/1", h, graph.Schema, rid)
	}
	if n, ok := r.Store.Node(graph.RepoNodeID(rid), rev); !ok || n.Kind != "repo" || n.Attrs["rid"] != rid || n.Attrs["ephemeral"] != "true" {
		t.Errorf("repo node = %+v, %v; want kind repo with the ephemeral id", n, ok)
	}
}

// TestBuild_UnitLangRequired (W-M1.4): a unit with no lang, or one of a
// language no resolver is registered for, is an error naming it.
func TestBuild_UnitLangRequired(t *testing.T) {
	root := shapes(t)
	_, err := Build(root, "c1", Config{Units: []Unit{{Path: "py", ModRoot: "py"}}})
	if err == nil || !strings.Contains(err.Error(), `"py"`) || !strings.Contains(err.Error(), "lang") {
		t.Errorf("no lang: err = %v; want one naming the unit and lang", err)
	}
	_, err = Build(root, "c1", Config{Units: []Unit{{Path: ".", Lang: "rust"}}})
	if err == nil || !strings.Contains(err.Error(), "rust") {
		t.Errorf("unknown lang: err = %v; want one naming rust", err)
	}
}

// TestBuild_UnitScopesItsLanguage (W-M1.4): a language's resolver reads
// only the files under that language's units; a file outside every unit of
// its language, and every file of an undeclared language, is not indexed.
func TestBuild_UnitScopesItsLanguage(t *testing.T) {
	root := shapes(t)
	r, err := Build(root, "c1", Config{Units: []Unit{{Path: "shape", Lang: gopg.Lang}}})
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit("c1")
	if _, ok := r.Store.Node("unit:go:shape", rev); !ok {
		t.Errorf("the declared unit's package is missing")
	}
	if _, ok := r.Store.Node("unit:go:app", rev); ok {
		t.Errorf("app indexed outside every Go unit")
	}
	if _, ok := r.Store.Node("file:py/app.py", rev); ok {
		t.Errorf("a Python file indexed with no Python unit")
	}
	if r.Files != 1 || r.Header.Producers["py"] != "" {
		t.Errorf("files = %d, producers = %v; want 1 and no Python producer", r.Files, r.Header.Producers)
	}
}

// TestBuild_GoNeedsASync (W-M1.4): a Go unit under a root with no pgraph
// sync is the producer's error, naming the remedy.
func TestBuild_GoNeedsASync(t *testing.T) {
	root := filepath.Join(t.TempDir(), "r")
	put(t, filepath.Join(root, "main.go"), "package main\n")
	_, err := Build(root, "c1", Config{Units: []Unit{{Path: ".", Lang: gopg.Lang}}})
	if !errors.Is(err, platform.ErrNotSynced) || !strings.Contains(err.Error(), "pgraph sync") {
		t.Errorf("err = %v; want ErrNotSynced naming pgraph sync", err)
	}
}

// TestIdentity_TwoRepos (W-M1.4, AC-X.4): two repositories with identical
// paths and symbol names index to the same local ids under distinct
// repository ids, so their global ids differ and split back to their own
// repository; a store holds local ids only.
func TestIdentity_TwoRepos(t *testing.T) {
	a, b := shapes(t), shapes(t)
	ra, err := Build(a, "c1", twoLangs)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := Build(b, "c1", twoLangs)
	if err != nil {
		t.Fatal(err)
	}
	if ra.Header.Repo == rb.Header.Repo || ra.Header.Repo == "" {
		t.Fatalf("repos = %q, %q; want two distinct ids", ra.Header.Repo, rb.Header.Repo)
	}
	rev := graph.RevCommit("c1")
	local := graph.ID("sym:go:shape:func:Sum")
	for _, r := range []Result{ra, rb} {
		if _, ok := r.Store.Node(local, rev); !ok {
			t.Errorf("%s has no %s", r.Header.Repo, local)
		}
		for _, n := range r.Store.Nodes(rev) {
			if strings.HasPrefix(string(n.ID), "g:") {
				t.Errorf("a store holds a global id %s", n.ID)
			}
		}
	}
	ga, gb := graph.GlobalID(ra.Header.Repo, local), graph.GlobalID(rb.Header.Repo, local)
	if ga == gb {
		t.Errorf("one global id for two repositories: %s", ga)
	}
	if rid, l, ok := graph.SplitGlobal(ga); !ok || rid != ra.Header.Repo || l != local {
		t.Errorf("SplitGlobal(%s) = %q, %q, %v", ga, rid, l, ok)
	}
}
