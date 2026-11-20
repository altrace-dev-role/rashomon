package pylite_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// relRev is the commit the relative-import fixtures are indexed at.
var relRev = graph.RevCommit("c1")

// extractFiles writes files under a temp root, extracts them all at relRev
// and returns the records with the store that holds them.
func extractFiles(t *testing.T, res pylite.Resolver, files map[string]string) ([]graph.Record, *graph.Store) {
	t.Helper()
	root := t.TempDir()
	rels := make([]string, 0, len(files))
	for rel, text := range files {
		writeFile(t, root, rel, text)
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	recs, err := res.Extract(root, rels, relRev)
	if err != nil {
		t.Fatal(err)
	}
	s := graph.NewStore()
	for _, r := range recs {
		if r.Node != nil {
			s.AssertNode(*r.Node)
		}
		if r.Edge != nil {
			s.AssertEdge(*r.Edge)
		}
	}
	return recs, s
}

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// edgesOf is "src -pred-> dst" for each edge in recs with pred, sorted.
func edgesOf(recs []graph.Record, pred graph.Pred) []string {
	var out []string
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Pred == pred {
			out = append(out, string(r.Edge.Src)+" -"+string(pred)+"-> "+string(r.Edge.Dst))
		}
	}
	sort.Strings(out)
	return out
}

// TestRelImport_EachFormResolvesAgainstThePackage: a from-import with
// leading dots names a module relative to the file's package (PEP 328): the
// package is the file's own module for __init__.py and its module less the
// last part otherwise; each dot past the first goes one package up. More
// dots than the package has parts is an import Python refuses, so no edge
// is written and a call of the name resolves to nothing. The resolved
// module then takes the path an absolute import takes: the imports edge,
// the references edge of a from-imported name, and a call's callee. The
// module map knows both "pkg" and "x", so a rule that dropped the dots or
// stopped at the root would write an edge the past-root rows forbid.
// Overlay of the same content writes the same three predicates.
func TestRelImport_EachFormResolvesAgainstThePackage(t *testing.T) {
	res := pylite.Resolver{
		Units:   []pylite.Unit{{Path: "src/pkg", ModRoot: "src"}, {Path: "top", ModRoot: "top"}},
		Modules: map[string]string{"pkg": "src/pkg", "x": "top"},
	}
	mod := func(name string) string { return string(graph.SymID(pylite.Lang, "src/pkg", "mod", name)) }
	fn := func(name string) string { return string(graph.SymID(pylite.Lang, "src/pkg", "func", name)) }
	call := "\n\ndef f():\n    y(1)\n"
	for _, c := range []struct {
		name, rel, src string
		imports, refs  []string // destinations from the file
		calls          []string // "caller callee"
	}{
		{"one dot and a module", "src/pkg/a.py", "from .x import y\n", []string{mod("pkg.x")}, []string{fn("pkg.x.y")}, nil},
		{"one dot alone", "src/pkg/a.py", "from . import x\n", []string{mod("pkg")}, []string{fn("pkg.x")}, nil},
		{"two dots and a module", "src/pkg/sub/a.py", "from ..x import y\n", []string{mod("pkg.x")}, []string{fn("pkg.x.y")}, nil},
		{"two dots alone", "src/pkg/sub/a.py", "from .. import x\n", []string{mod("pkg")}, []string{fn("pkg.x")}, nil},
		{"past the root of a package", "src/pkg/a.py", "from ..x import y" + call, nil, nil, nil},
		{"a top-level module has no package", "top/a.py", "from .x import y" + call, nil, nil, nil},
		{"inside __init__.py the package is the module", "src/pkg/__init__.py", "from .x import y\n", []string{mod("pkg.x")}, []string{fn("pkg.x.y")}, nil},
		{"a call through a relative import", "src/pkg/a.py", "from .x import y" + call, []string{mod("pkg.x")}, []string{fn("pkg.x.y")},
			[]string{fn("pkg.a.f") + " " + fn("pkg.x.y")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			recs, _ := extractFiles(t, res, map[string]string{c.rel: c.src})
			file := string(graph.FileID(c.rel))
			want := func(pred graph.Pred, dsts []string) []string {
				var out []string
				for _, d := range dsts {
					out = append(out, file+" -"+string(pred)+"-> "+d)
				}
				sort.Strings(out)
				return out
			}
			var calls []string
			for _, cl := range c.calls {
				src, dst, _ := strings.Cut(cl, " ")
				calls = append(calls, src+" -"+string(graph.Calls)+"-> "+dst)
			}
			for _, chk := range []struct {
				pred graph.Pred
				want []string
			}{{graph.Imports, want(graph.Imports, c.imports)}, {graph.References, want(graph.References, c.refs)}, {graph.Calls, calls}} {
				if got := edgesOf(recs, chk.pred); !slices.Equal(got, chk.want) {
					t.Errorf("%s edges %q; want %q", chk.pred, got, chk.want)
				}
			}
			ov, err := res.Overlay(c.rel, []byte(c.src), relRev, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []graph.Pred{graph.Imports, graph.References, graph.Calls} {
				if got, want := edgesOf(ov, p), edgesOf(recs, p); !slices.Equal(got, want) {
					t.Errorf("overlay %s edges %q; extract %q", p, got, want)
				}
			}
		})
	}
}

// clickRows is, for each relative from-statement outside def bodies in
// click's src/click at fe307c1e, the importing file, the form as written
// (dots and module) and the file of the module CPython's
// importlib.util.resolve_name names: 55 rows, 55 file-to-module pairs, 15
// target files. Only the layout and the forms are kept, not the names
// imported or any other text of the project.
const clickRows = `
src/click/__init__.py         .core             src/click/core.py
src/click/__init__.py         .decorators       src/click/decorators.py
src/click/__init__.py         .exceptions       src/click/exceptions.py
src/click/__init__.py         .formatting       src/click/formatting.py
src/click/__init__.py         .globals          src/click/globals.py
src/click/__init__.py         .termui           src/click/termui.py
src/click/__init__.py         .types            src/click/types.py
src/click/__init__.py         .utils            src/click/utils.py
src/click/_compat.py          ._winconsole      src/click/_winconsole.py
src/click/_termui_impl.py     ._compat          src/click/_compat.py
src/click/_termui_impl.py     .exceptions       src/click/exceptions.py
src/click/_termui_impl.py     .utils            src/click/utils.py
src/click/_textwrap.py        ._compat          src/click/_compat.py
src/click/_winconsole.py      ._compat          src/click/_compat.py
src/click/core.py             .                 src/click/__init__.py
src/click/core.py             ._utils           src/click/_utils.py
src/click/core.py             .exceptions       src/click/exceptions.py
src/click/core.py             .formatting       src/click/formatting.py
src/click/core.py             .globals          src/click/globals.py
src/click/core.py             .parser           src/click/parser.py
src/click/core.py             .shell_completion src/click/shell_completion.py
src/click/core.py             .termui           src/click/termui.py
src/click/core.py             .utils            src/click/utils.py
src/click/decorators.py       .core             src/click/core.py
src/click/decorators.py       .globals          src/click/globals.py
src/click/decorators.py       .utils            src/click/utils.py
src/click/exceptions.py       ._compat          src/click/_compat.py
src/click/exceptions.py       .core             src/click/core.py
src/click/exceptions.py       .globals          src/click/globals.py
src/click/exceptions.py       .utils            src/click/utils.py
src/click/formatting.py       ._compat          src/click/_compat.py
src/click/formatting.py       .parser           src/click/parser.py
src/click/globals.py          .core             src/click/core.py
src/click/parser.py           ._utils           src/click/_utils.py
src/click/parser.py           .core             src/click/core.py
src/click/parser.py           .exceptions       src/click/exceptions.py
src/click/shell_completion.py .core             src/click/core.py
src/click/shell_completion.py .utils            src/click/utils.py
src/click/termui.py           .                 src/click/__init__.py
src/click/termui.py           ._compat          src/click/_compat.py
src/click/termui.py           ._termui_impl     src/click/_termui_impl.py
src/click/termui.py           .exceptions       src/click/exceptions.py
src/click/termui.py           .globals          src/click/globals.py
src/click/termui.py           .types            src/click/types.py
src/click/termui.py           .utils            src/click/utils.py
src/click/testing.py          .                 src/click/__init__.py
src/click/testing.py          ._compat          src/click/_compat.py
src/click/testing.py          .core             src/click/core.py
src/click/types.py            ._compat          src/click/_compat.py
src/click/types.py            .core             src/click/core.py
src/click/types.py            .exceptions       src/click/exceptions.py
src/click/types.py            .shell_completion src/click/shell_completion.py
src/click/types.py            .utils            src/click/utils.py
src/click/utils.py            ._compat          src/click/_compat.py
src/click/utils.py            .globals          src/click/globals.py
`

// TestRelImport_ClickImportersReachTheirModulesFiles: over click's 55
// relative-import rows, each importer is written with one line "from
// <form> import n<i>" per row (a name per row, as imports are kept by
// name), and every target file is present. Each row's target file is in
// the importer's depth-2 neighborhood, the walk the prompt brief ranks: the
// imports edge reaches the module id, and the file that is that module
// declares it. Without the declaration a row is reached only when the two
// files import a module in common.
func TestRelImport_ClickImportersReachTheirModulesFiles(t *testing.T) {
	res := pylite.Resolver{Units: []pylite.Unit{{Path: "src/click", ModRoot: "src"}}, Modules: map[string]string{"click": "src/click"}}
	type row struct{ imp, form, tgt string }
	var rows []row
	files := map[string]string{}
	for i, line := range strings.Split(strings.TrimSpace(clickRows), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			t.Fatalf("row %d %q: want three fields", i+1, line)
		}
		rows = append(rows, row{f[0], f[1], f[2]})
		files[f[0]] += fmt.Sprintf("from %s import n%d\n", f[1], i+1)
		if _, ok := files[f[2]]; !ok {
			files[f[2]] = ""
		}
	}
	if len(rows) != 55 {
		t.Fatalf("%d rows; want 55", len(rows))
	}
	recs, s := extractFiles(t, res, files)
	imports := len(edgesOf(recs, graph.Imports))
	reached, targets := 0, map[string]bool{}
	for _, r := range rows {
		nb := engine.Traverse(s, []graph.ID{graph.FileID(r.imp)}, engine.Depth, relRev)
		if _, ok := nb.Distance[graph.FileID(r.tgt)]; ok {
			reached++
			targets[r.tgt] = true
		}
	}
	t.Logf("imports edges %d, rows reached %d of %d, targets %d", imports, reached, len(rows), len(targets))
	if imports != 55 || reached != 55 || len(targets) != 15 {
		t.Errorf("imports edges %d, rows reached %d of 55, targets %d; want 55, 55, 15", imports, reached, len(targets))
	}
}

// TestRelImport_EachFileDeclaresItsModule: a parsed file declares the id
// an import of its module ends at, sym:py:<its unit>:mod:<its module>, so
// the importer reaches it at depth 2; __init__.py is its package's module.
// An overlay writes the same declaration, as Declares is carried.
func TestRelImport_EachFileDeclaresItsModule(t *testing.T) {
	res := pylite.Resolver{Units: []pylite.Unit{{Path: "src/pkg", ModRoot: "src"}}, Modules: map[string]string{"pkg": "src/pkg"}}
	files := map[string]string{"src/pkg/__init__.py": "", "src/pkg/a.py": "def g():\n    return 1\n"}
	recs, _ := extractFiles(t, res, files)
	for rel, name := range map[string]string{"src/pkg/__init__.py": "pkg", "src/pkg/a.py": "pkg.a"} {
		want := string(graph.FileID(rel)) + " -" + string(graph.Declares) + "-> " + string(graph.SymID(pylite.Lang, "src/pkg", "mod", name))
		if got := edgesOf(recs, graph.Declares); !slices.Contains(got, want) {
			t.Errorf("extract declares %q; want %q among them", got, want)
		}
		ov, err := res.Overlay(rel, []byte(files[rel]), relRev, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := edgesOf(ov, graph.Declares); !slices.Contains(got, want) {
			t.Errorf("overlay declares %q; want %q among them", got, want)
		}
	}
}

// TestRelImport_AnImporterInAnotherUnitIsAConsumer: a file of unit a that
// imports a module of unit b is a consumer line of b's unit brief, as a Go
// file importing a package is. Control: a file of unit a calling a
// function of unit b through a from-import is one already, so a missing
// line is the import's and not the fixture's.
func TestRelImport_AnImporterInAnotherUnitIsAConsumer(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/app/importer.py", "import lib.m\n")
	writeFile(t, root, "a/app/caller.py", "from lib.m import helper\n\n\ndef run():\n    helper()\n")
	writeFile(t, root, "b/lib/m.py", "def helper():\n    return 1\n")
	r, err := index.Build(root, "c1", index.Config{Units: []index.Unit{
		{Path: "a", ModRoot: "a", Modules: []string{"app"}, Lang: pylite.Lang},
		{Path: "b", ModRoot: "b", Modules: []string{"lib"}, Lang: pylite.Lang},
	}})
	if err != nil {
		t.Fatal(err)
	}
	b := engine.UnitBrief(r.Store, relRev, graph.UnitID(pylite.Lang, "b"), engine.Brief{}, engine.Options{})
	var consumers []string
	for _, l := range b.Lines {
		if l.Role == engine.RoleConsumer {
			consumers = append(consumers, string(l.Key))
		}
	}
	sort.Strings(consumers)
	if want := []string{"file:a/app/caller.py", "file:a/app/importer.py"}; !slices.Equal(consumers, want) {
		t.Errorf("consumer lines of unit b %q; want %q", consumers, want)
	}
}
