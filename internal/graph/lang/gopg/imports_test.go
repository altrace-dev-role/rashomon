package gopg

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// rootModule is the module path of the repository root at the M2 tip; the
// nested pgraph module's path is it plus "/pgraph" (pgraph/go.mod).
const rootModule = "github.com/altrace-dev-role/rashomon"

// dirSet is an isDir over a fixed set of directories that counts its calls.
type dirSet struct {
	dirs  map[string]bool
	calls []string
}

func newDirSet(dirs ...string) *dirSet {
	d := &dirSet{dirs: map[string]bool{}}
	for _, x := range dirs {
		d.dirs[x] = true
	}
	return d
}

func (d *dirSet) isDir(dir string) bool {
	d.calls = append(d.calls, dir)
	return d.dirs[dir]
}

// TestImportUnit_MapsUnderTheModuleOnly (record line 106, 01:07 clarification
// 1): an import path maps to unit:go:<dir> when it is the module path or under
// it by a slash and the directory exists. A nested module whose path follows
// its directory (pgraph) maps by the same rule; a module whose path does not
// (example.test/mod in pgraph/testdata), the standard library and a
// third-party module map to nothing. isDir is asked only about a candidate.
func TestImportUnit_MapsUnderTheModuleOnly(t *testing.T) {
	cases := []struct {
		module, imp string
		want        graph.ID
		asked       bool
	}{
		{rootModule, rootModule, "unit:go:.", true},
		{rootModule, rootModule + "/internal/graph", "unit:go:internal/graph", true},
		{rootModule, rootModule + "/pgraph", "unit:go:pgraph", true},
		{rootModule, rootModule + "/pgraph/cmd/pgraph", "unit:go:pgraph/cmd/pgraph", true},
		{rootModule, rootModule + "/internal/missing", "", true},
		{rootModule, "example.test/mod", "", false},
		{rootModule, "fmt", "", false},
		{rootModule, "golang.org/x/tools/go/packages", "", false},
		{rootModule, rootModule + "x/internal/graph", "", false},
		{rootModule, "", "", false},
		{rootModule, rootModule + "/", "", false},
		{rootModule, rootModule + "//internal/graph", "", false},
		{rootModule, rootModule + "/internal/../pgraph", "", false},
		{rootModule, rootModule + "/./internal/graph", "", false},
		{"", rootModule + "/internal/graph", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		d := newDirSet(".", "internal/graph", "pgraph", "pgraph/cmd/pgraph")
		got, ok := ImportUnit(c.module, c.imp, d.isDir)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("ImportUnit(%q, %q) = %q, %v; want %q", c.module, c.imp, got, ok, c.want)
		}
		if asked := len(d.calls) > 0; asked != c.asked {
			t.Errorf("ImportUnit(%q, %q) asked isDir %v; want asked=%v", c.module, c.imp, d.calls, c.asked)
		}
		if len(d.calls) > 1 {
			t.Errorf("ImportUnit(%q, %q) asked isDir %d times", c.module, c.imp, len(d.calls))
		}
	}
}

// TestModulePath_ReadsTheModuleDirective: the module directive's path, bare,
// quoted or back-quoted, after comments and blank lines, with a trailing
// comment or CR; "" with no directive or a malformed one.
func TestModulePath_ReadsTheModuleDirective(t *testing.T) {
	cases := map[string]string{
		"module example.com/x\n\ngo 1.25.0\n":               "example.com/x",
		"// lead\n\nmodule \"example.com/q\" // trailing\n": "example.com/q",
		"module `example.com/b`\n":                          "example.com/b",
		"  module\texample.com/sp  \n":                      "example.com/sp",
		"module example.com/crlf\r\ngo 1.25\r\n":            "example.com/crlf",
		"go 1.25.0\n":                                       "",
		"modulex example.com/no\n":                          "",
		"module\n":                                          "",
		"module a b\n":                                      "",
		"module \"unterminated\n":                           "",
		"":                                                  "",
		"// module example.com/commented\nmodule example.com/r\n": "example.com/r",
	}
	for in, want := range cases {
		if got := ModulePath([]byte(in)); got != want {
			t.Errorf("ModulePath(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestReadModulePath_RootGoModCapped: the root's go.mod is read through the
// cap; a directive past it, and a root with no go.mod, give "".
func TestReadModulePath_RootGoModCapped(t *testing.T) {
	root := t.TempDir()
	if got := ReadModulePath(root); got != "" {
		t.Fatalf("no go.mod: %q", got)
	}
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("module example.com/root\n\ngo 1.25.0\n")
	if got := ReadModulePath(root); got != "example.com/root" {
		t.Fatalf("go.mod: %q", got)
	}
	write("// " + strings.Repeat("x", MaxGoModBytes) + "\nmodule example.com/late\n")
	if got := ReadModulePath(root); got != "" {
		t.Fatalf("directive past the cap read: %q", got)
	}
	line := "module example.com/straddle\n"
	write("// " + strings.Repeat("x", MaxGoModBytes-len(line)/2-4) + "\n" + line)
	if got := ReadModulePath(root); got != "" {
		t.Fatalf("a directive cut by the cap read as %q", got)
	}
}

// TestDirUnder_DirectoriesOnlyNoLink: a directory under root is one; a file,
// a missing path and a link to a directory are not.
func TestDirUnder_DirectoriesOnlyNoLink(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a/b", "real"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a", "f.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	isDir := DirUnder(root)
	for dir, want := range map[string]bool{".": true, "a": true, "a/b": true, "real": true, "a/f.go": false, "missing": false, "link": false} {
		if got := isDir(dir); got != want {
			t.Errorf("DirUnder(root)(%q) = %v; want %v", dir, got, want)
		}
	}
}

// importsOf is the Imports edges of recs, by key.
func importsOf(recs []graph.Record) map[string]graph.Edge {
	out := map[string]graph.Edge{}
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Pred == graph.Imports {
			out[r.Edge.Key()] = *r.Edge
		}
	}
	return out
}

func overlayWith(t *testing.T, m *ImportMap, rel, src string) []graph.Record {
	t.Helper()
	recs, err := Resolver{Imports: m}.Overlay(rel, []byte(src), overlayBase, 3)
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	return recs
}

const crossSrc = `package hook

import (
	"fmt"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	pg "github.com/altrace-dev-role/rashomon/pgraph"
	_ "github.com/altrace-dev-role/rashomon/internal/graph"
	"golang.org/x/tools/go/packages"
	"example.test/mod"
)

var _ = fmt.Sprint
`

// TestOverlay_EmitsOneImportsEdgePerMappedDir (D1, D4): one Imports edge from
// the file to each distinct mapped directory, at the overlay's tier,
// confidence, revision and provenance; nothing for the standard library, a
// third-party module or a module outside the root's path. The nested pgraph
// module maps from the import line alone: no go.mod is read here, so no
// require is consulted (clarification 3).
func TestOverlay_EmitsOneImportsEdgePerMappedDir(t *testing.T) {
	d := newDirSet("internal/graph", "pgraph", "internal/hook")
	recs := overlayWith(t, &ImportMap{Module: rootModule, IsDir: d.isDir}, "internal/hook/x.go", crossSrc)
	got := importsOf(recs)
	file := graph.FileID("internal/hook/x.go")
	want := []string{key(file, graph.Imports, "unit:go:internal/graph"), key(file, graph.Imports, "unit:go:pgraph")}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, want) {
		t.Fatalf("Imports edges %q; want %q", keys, want)
	}
	rev := graph.Rev{Kind: graph.RevKindOverlay, Commit: overlayBase.Commit, Seq: 3}
	for k, e := range got {
		if e.Tier != graph.Algorithmic || e.Conf != overlayConf || e.Rev != rev || !reflect.DeepEqual(e.Prov, overlayProv) {
			t.Errorf("%q: tier %s conf %v rev %+v prov %+v", k, e.Tier, e.Conf, e.Rev, e.Prov)
		}
	}
	if n := countOf(d.calls, "internal/graph"); n != 1 {
		t.Errorf("isDir asked about internal/graph %d times; want once per distinct directory (%v)", n, d.calls)
	}
}

// TestOverlay_NilImportsEmitsNone: the zero Resolver (no ImportMap) and a map
// with no module emit no Imports edge, as before this card.
func TestOverlay_NilImportsEmitsNone(t *testing.T) {
	for _, m := range []*ImportMap{nil, {Module: "", IsDir: newDirSet("internal/graph", "pgraph").isDir}} {
		if got := importsOf(overlayWith(t, m, "internal/hook/x.go", crossSrc)); len(got) != 0 {
			t.Errorf("ImportMap %+v: Imports edges %v; want none", m, got)
		}
	}
}

// TestOverlay_TestFileEmitsNoImports (D5, clarification 5): the index holds no
// _test.go file (pgraph loads ./... without tests), so the overlay emits no
// Imports edge from one; its declarations are unchanged.
func TestOverlay_TestFileEmitsNoImports(t *testing.T) {
	d := newDirSet("internal/graph", "pgraph")
	recs := overlayWith(t, &ImportMap{Module: rootModule, IsDir: d.isDir}, "internal/hook/x_test.go", crossSrc)
	if got := importsOf(recs); len(got) != 0 {
		t.Fatalf("a _test.go file gave Imports edges %v", got)
	}
	if len(d.calls) != 0 {
		t.Errorf("a _test.go file asked isDir %v", d.calls)
	}
	if !slices.ContainsFunc(recs, func(r graph.Record) bool { return r.Node != nil && r.Node.ID == "file:internal/hook/x_test.go" }) {
		t.Errorf("a _test.go file lost its file node")
	}
}

// TestOverlay_ImportOfOwnDirIsNoEdge (D7): a mapping that gives the file's own
// unit emits no edge.
func TestOverlay_ImportOfOwnDirIsNoEdge(t *testing.T) {
	src := "package graph\n\nimport (\n\t\"github.com/altrace-dev-role/rashomon/internal/graph\"\n\t\"github.com/altrace-dev-role/rashomon/pgraph\"\n)\n"
	got := importsOf(overlayWith(t, &ImportMap{Module: rootModule, IsDir: newDirSet("internal/graph", "pgraph").isDir}, "internal/graph/x.go", src))
	want := []string{key(graph.FileID("internal/graph/x.go"), graph.Imports, "unit:go:pgraph")}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, want) {
		t.Fatalf("Imports edges %q; want %q", keys, want)
	}
}

// TestOverlay_ImportsCapped (D9, RES-29): at most MaxOverlayImports import
// specs are mapped, so a file of many imports costs a bounded number of
// directory checks and edges.
func TestOverlay_ImportsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("package hook\n\nimport (\n")
	var dirs []string
	for i := range 3 * MaxOverlayImports {
		dir := fmt.Sprintf("internal/p%03d", i)
		dirs = append(dirs, dir)
		fmt.Fprintf(&b, "\t_ %q\n", rootModule+"/"+dir)
	}
	b.WriteString(")\n")
	d := newDirSet(dirs...)
	got := importsOf(overlayWith(t, &ImportMap{Module: rootModule, IsDir: d.isDir}, "internal/hook/x.go", b.String()))
	if len(got) != MaxOverlayImports || len(d.calls) != MaxOverlayImports {
		t.Fatalf("%d edges and %d isDir calls over %d imports; want %d each", len(got), len(d.calls), 3*MaxOverlayImports, MaxOverlayImports)
	}
}

// TestOverlayCarried_IsDeclaresAndImports (D8): an overlay that maps imports
// is authoritative on its file's declarations and imports; one that cannot
// (no map, no module, no IsDir) carries Declares alone, so the index's
// Imports edges of the file stand rather than being retracted unreplaced.
func TestOverlayCarried_IsDeclaresAndImports(t *testing.T) {
	isDir := newDirSet("pgraph").isDir
	cases := []struct {
		m    *ImportMap
		want []graph.Pred
	}{
		{&ImportMap{Module: rootModule, IsDir: isDir}, []graph.Pred{graph.Declares, graph.Imports}},
		{nil, []graph.Pred{graph.Declares}},
		{&ImportMap{Module: "", IsDir: isDir}, []graph.Pred{graph.Declares}},
		{&ImportMap{Module: rootModule}, []graph.Pred{graph.Declares}},
	}
	for _, c := range cases {
		if got := slices.Sorted(slices.Values(Resolver{Imports: c.m}.OverlayCarried())); !slices.Equal(got, c.want) {
			t.Errorf("ImportMap %+v: OverlayCarried = %v; want %v", c.m, got, c.want)
		}
	}
}

// TestExtract_ImportEdgesMatchTheOverlays (D1, D2, D4): the index reads the
// root's go.mod and each synced file's import clause with the overlay's own
// mapping, so an unchanged file's overlay gives exactly the index's Imports
// keys and diffs to none. The index edge is ALGORITHMIC at 1.0 under
// go/parser's provenance, appended last, so the first record and the header's
// producer stay pgraph's.
func TestExtract_ImportEdgesMatchTheOverlays(t *testing.T) {
	root := shapes(t)
	recs, err := Resolver{}.Extract(root, nil, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	if p := firstProv(recs); !reflect.DeepEqual(p, prov) {
		t.Fatalf("first record's prov %+v; want pgraph's %+v", p, prov)
	}
	idx := importsOf(recs)
	file := graph.FileID("app/app.go")
	want := key(file, graph.Imports, "unit:go:shape")
	if keys := slices.Sorted(maps.Keys(idx)); !slices.Equal(keys, []string{want}) {
		t.Fatalf("index Imports edges %q; want [%q]", keys, want)
	}
	if e := idx[want]; e.Tier != graph.Algorithmic || e.Conf != 1.0 || !reflect.DeepEqual(e.Prov, overlayProv) || e.Rev != graph.RevCommit("c1") {
		t.Errorf("index edge tier %s conf %v prov %+v rev %+v", e.Tier, e.Conf, e.Prov, e.Rev)
	}
	for i := len(recs) - len(idx); i < len(recs); i++ {
		if recs[i].Edge == nil || recs[i].Edge.Pred != graph.Imports {
			t.Fatalf("Imports edges are not the last %d records (record %d)", len(idx), i)
		}
	}
	src, err := os.ReadFile(filepath.Join(root, "app", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	m := &ImportMap{Module: ReadModulePath(root), IsDir: DirUnder(root)}
	ov := importsOf(overlayWith(t, m, "app/app.go", string(src)))
	if a, b := slices.Sorted(maps.Keys(ov)), slices.Sorted(maps.Keys(idx)); !slices.Equal(a, b) {
		t.Fatalf("overlay Imports keys %q; index %q", a, b)
	}
}

func firstProv(recs []graph.Record) graph.Prov {
	for _, r := range recs {
		switch {
		case r.Node != nil && r.Node.Prov.Extractor != "":
			return r.Node.Prov
		case r.Edge != nil && r.Edge.Prov.Extractor != "":
			return r.Edge.Prov
		}
	}
	return graph.Prov{}
}

func countOf(xs []string, x string) int {
	n := 0
	for _, y := range xs {
		if y == x {
			n++
		}
	}
	return n
}
