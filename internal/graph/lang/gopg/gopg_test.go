package gopg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// shapes is the fixture the branch pgraph synced: two packages, an
// interface, a pointer implementer, an embedder, a dispatching call, a call
// into the standard library, documented and undocumented declarations.
func shapes(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "shapes"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// split is the records as nodes by id and edges by key.
func split(recs []graph.Record) (map[graph.ID]graph.Node, map[string]graph.Edge) {
	nodes, edges := map[graph.ID]graph.Node{}, map[string]graph.Edge{}
	for _, r := range recs {
		switch {
		case r.Node != nil:
			nodes[r.Node.ID] = *r.Node
		case r.Edge != nil:
			edges[r.Edge.Key()] = *r.Edge
		}
	}
	return nodes, edges
}

func key(src graph.ID, p graph.Pred, dst graph.ID) string {
	return graph.Edge{Src: src, Pred: p, Dst: dst}.Key()
}

// TestGopg_UnitIsDirectory (W-M1.3, AD2): a Go unit is the package
// directory under the root, "." at the root; every .go file is claimed.
func TestGopg_UnitIsDirectory(t *testing.T) {
	var r Resolver
	for rel, want := range map[string]graph.ID{
		"internal/platform/snapshot.go": "unit:go:internal/platform",
		"main.go":                       "unit:go:.",
		"pgraph/cmd/pgraph/main.go":     "unit:go:pgraph/cmd/pgraph",
	} {
		if got, ok := r.Unit("", rel); !ok || got != want {
			t.Errorf("Unit(%q) = %q, %v; want %q", rel, got, ok, want)
		}
	}
	if !r.Claims("", "a/b.go") || r.Claims("", "a/b.py") || r.Claims("", "a/b.go.txt") || r.Lang() != "go" {
		t.Errorf("Claims or Lang wrong")
	}
	if recs, err := r.Overlay("a/b.go", []byte("package b\n"), graph.RevCommit("c1"), 1); recs != nil || err != nil {
		t.Errorf("Overlay = %v, %v; want nil, nil (admitted without overlays)", recs, err)
	}
}

// TestGopg_RecordsForFixture (W-M1.3, AC-M1.0): the records equal the
// sidecars: a unit per package directory, a file node per digests row with
// that row's digest, a sym node per symbols row (and one for the interface
// method the facts name and no row places), and the edges of every fact
// whose two ends are in the sync; the standard-library callee is dropped.
func TestGopg_RecordsForFixture(t *testing.T) {
	root := shapes(t)
	s, err := platform.ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Resolver{}.Extract(root, nil, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	nodes, edges := split(recs)
	count := func(kind string) int {
		n := 0
		for _, x := range nodes {
			if x.Kind == kind {
				n++
			}
		}
		return n
	}
	files := 0
	for pkg, fs := range s.Files {
		files += len(fs)
		for _, rel := range fs {
			n, ok := nodes[graph.FileID(rel)]
			if !ok || n.Attrs["digest"] != s.Digests[rel] || n.Attrs["pkg"] != pkg || n.Tier != graph.Proven {
				t.Errorf("file %s = %+v; want the digests row's digest %s under %s", rel, n, s.Digests[rel], pkg)
			}
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil || graph.ContentDigest(b) != n.Attrs["digest"] {
				t.Errorf("file %s: digest attr %q is not ContentDigest of the file (%v)", rel, n.Attrs["digest"], err)
			}
		}
	}
	if count("unit") != 2 || count("file") != files || count("sym") != len(s.Places)+1 {
		t.Errorf("units %d, files %d, syms %d; want 2, %d, %d", count("unit"), count("file"), count("sym"), files, len(s.Places)+1)
	}
	preds := map[graph.Pred]int{}
	for _, e := range edges {
		preds[e.Pred]++
	}
	want := map[graph.Pred]int{graph.Contains: 2, graph.Declares: len(s.Places) + 2, graph.Calls: 2, graph.MayCall: 1, graph.ResolvesTo: 2, graph.References: 1}
	for p, n := range want {
		if preds[p] != n {
			t.Errorf("%s edges = %d; want %d", p, preds[p], n)
		}
	}
	for k := range edges {
		if _, has := want[graph.Pred(strings.Split(k, "\x00")[1])]; !has {
			t.Errorf("unexpected edge %q", k)
		}
	}
	area := nodes["sym:go:shape:method:Square.Area"]
	if area.Attrs["name"] != "Area" || area.Attrs["symkind"] != "method" || area.Attrs["line"] != "10" || area.Attrs["doc"] != "Area is the square's area." || area.Attrs["pkg"] != "example.com/shapes/shape" {
		t.Errorf("Square.Area = %+v", area.Attrs)
	}
	if d := nodes["sym:go:app:var:Default"]; d.Kind != "sym" || d.Attrs["doc"] != "" || d.Attrs["line"] != "17" {
		t.Errorf("Default = %+v; want a sym at line 17 with no doc", d)
	}
	if n := nodes["sym:go:shape:method:Shape.Area"]; n.Kind != "sym" || n.Attrs["line"] != "" {
		t.Errorf("the interface method = %+v; want an unplaced sym", n)
	}
	for _, k := range []string{
		key("unit:go:shape", graph.Contains, "file:shape/shape.go"),
		key("file:shape/shape.go", graph.Declares, "sym:go:shape:method:Square.Area"),
		key("sym:go:shape:type:Square", graph.Declares, "sym:go:shape:method:Square.Area"),
		key("unit:go:shape", graph.Declares, "sym:go:shape:method:Shape.Area"),
		key("sym:go:app:func:Run", graph.Calls, "sym:go:shape:func:Sum"),
		key("sym:go:shape:func:Sum", graph.Calls, "sym:go:shape:method:Shape.Area"),
		key("sym:go:shape:func:Sum", graph.MayCall, "sym:go:shape:method:Square.Area"),
		key("sym:go:app:type:Holder", graph.References, "sym:go:shape:type:Shape"),
		key("sym:go:app:type:Holder", graph.ResolvesTo, "sym:go:shape:type:Shape"),
		key("sym:go:shape:type:Square", graph.ResolvesTo, "sym:go:shape:type:Shape"),
	} {
		if _, ok := edges[k]; !ok {
			t.Errorf("no edge %q", strings.ReplaceAll(k, "\x00", " "))
		}
	}
	for _, r := range recs {
		var s string
		if r.Node != nil {
			s = string(r.Node.ID)
		} else {
			s = r.Edge.Key()
		}
		if strings.Contains(s, "strconv") {
			t.Errorf("a record names the standard library: %q", s)
		}
	}
}

// TestGopg_TiersPreserved (W-M1.3, AD4): a PROVEN call is a certain Calls
// edge; a may-call is a MayCall edge at the algorithmic tier, never certain
// and never a Calls edge; implements-via-pointer says so.
func TestGopg_TiersPreserved(t *testing.T) {
	recs, err := Resolver{}.Extract(shapes(t), nil, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	st := graph.NewStore()
	for _, r := range recs {
		switch {
		case r.Node != nil:
			st.AssertNode(*r.Node)
		case r.Edge != nil:
			st.AssertEdge(*r.Edge)
		}
	}
	at := graph.RevCommit("c1")
	if e, ok := st.Edge("sym:go:app:func:Run", graph.Calls, "sym:go:shape:func:Sum", at); !ok || e.Tier != graph.Proven || !e.Certain || e.Conf != 1.0 {
		t.Errorf("Run calls Sum = %+v, %v; want proven, certain", e, ok)
	}
	if e, ok := st.Edge("sym:go:shape:func:Sum", graph.MayCall, "sym:go:shape:method:Square.Area", at); !ok || e.Tier != graph.Algorithmic || e.Certain || e.Conf >= 1.0 {
		t.Errorf("Sum may_call Square.Area = %+v, %v; want algorithmic, uncertain", e, ok)
	}
	if _, ok := st.Edge("sym:go:shape:func:Sum", graph.Calls, "sym:go:shape:method:Square.Area", at); ok {
		t.Errorf("a may-call row became a Calls edge")
	}
	if e, ok := st.Edge("sym:go:shape:type:Square", graph.ResolvesTo, "sym:go:shape:type:Shape", at); !ok || e.Attrs["via"] != "pointer" || !e.Certain {
		t.Errorf("Square resolves_to Shape = %+v, %v; want via pointer, certain", e, ok)
	}
	if e, ok := st.Edge("sym:go:app:type:Holder", graph.ResolvesTo, "sym:go:shape:type:Shape", at); !ok || e.Attrs["via"] != "" {
		t.Errorf("Holder resolves_to Shape = %+v, %v; want no via", e, ok)
	}
}

// TestGopg_FilesRestrict (W-M1.3): Extract over a subset of files keeps
// those files, their symbols and the edges between them, and nothing of a
// package with no kept file.
func TestGopg_FilesRestrict(t *testing.T) {
	recs, err := Resolver{}.Extract(shapes(t), []string{"shape/shape.go"}, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	nodes, edges := split(recs)
	for id := range nodes {
		if strings.Contains(string(id), ":app") || strings.Contains(string(id), "app/") {
			t.Errorf("node %s from the unkept package", id)
		}
	}
	for k := range edges {
		if strings.Contains(k, ":app") {
			t.Errorf("edge %q touches the unkept package", strings.ReplaceAll(k, "\x00", " "))
		}
	}
	if _, ok := nodes["sym:go:shape:func:Sum"]; !ok {
		t.Errorf("Sum missing from the kept package")
	}
	if _, ok := edges[key("sym:go:shape:func:Sum", graph.MayCall, "sym:go:shape:method:Square.Area")]; !ok {
		t.Errorf("the kept package's may_call missing")
	}
}

// TestGopg_ErrorsWithoutSync (W-M1.3): a root with no sync is an error
// that names the remedy and wraps platform.ErrNotSynced.
func TestGopg_ErrorsWithoutSync(t *testing.T) {
	_, err := Resolver{}.Extract(t.TempDir(), nil, graph.RevCommit("c1"))
	if !errors.Is(err, platform.ErrNotSynced) || !strings.Contains(err.Error(), "pgraph sync") {
		t.Errorf("err = %v; want ErrNotSynced naming pgraph sync", err)
	}
}
