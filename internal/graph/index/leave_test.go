package index

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// envWhy is the reason pylite gives for a directory holding pyvenv.cfg.
const envWhy = lang.LeftEnvironment + " (holds pyvenv.cfg)"

// pyUnit declares py as one Python unit owning the module app.
var pyUnit = Config{Units: []Unit{{Path: "py", ModRoot: "py", Modules: []string{"app"}, Lang: pylite.Lang}}}

// TestBuild_AnEnvironmentIsLeftOutAndNamed (L2): under a configured Python
// unit, Build's walk leaves out each directory holding pyvenv.cfg, so no
// file under one is handed to the resolver or held, and names each, by
// path, in the result and in the header, count and names. Break: walk an
// environment; leave it out unnamed; name it in walk order (py/a/env walks
// before py/a-b, which sorts first).
func TestBuild_AnEnvironmentIsLeftOutAndNamed(t *testing.T) {
	root := wireRoot(t, map[string]string{
		"py/app/__init__.py": "",
		"py/app/a.py":        "def run():\n    return 1\n",
		"py/venv/pyvenv.cfg": "home = /usr/bin\n",
		"py/venv/lib/python3.12/site-packages/dep/__init__.py": "def f():\n    return 2\n",
		"py/venv/src/edit/edit/m.py":                           "x = 1\n",
		"py/a/env/pyvenv.cfg":                                  "home = /usr/bin\n",
		"py/a/env/lib/m.py":                                    "y = 1\n",
		"py/a-b/pyvenv.cfg":                                    "home = /usr/bin\n",
		"py/a-b/lib/m.py":                                      "z = 1\n",
	})
	r, err := Build(root, "c1", pyUnit)
	if err != nil {
		t.Fatal(err)
	}
	want := []graph.LeftDir{{Path: "py/a-b", Why: envWhy}, {Path: "py/a/env", Why: envWhy}, {Path: "py/venv", Why: envWhy}}
	if !reflect.DeepEqual(r.Left, want) {
		t.Errorf("Result.Left = %+v; want %+v", r.Left, want)
	}
	if r.Header.LeftOut != len(want) || !reflect.DeepEqual(r.Header.LeftOutDirs, want) {
		t.Errorf("header left out %d, named %+v; want %d, %+v", r.Header.LeftOut, r.Header.LeftOutDirs, len(want), want)
	}
	if r.Files != 2 {
		t.Errorf("files = %d; want the two files of package app", r.Files)
	}
	rev := graph.RevCommit("c1")
	if _, ok := r.Store.Node("file:py/app/a.py", rev); !ok {
		t.Errorf("witness: file:py/app/a.py is not held")
	}
	if _, ok := r.Store.Node("file:py/venv/lib/python3.12/site-packages/dep/__init__.py", rev); ok {
		t.Errorf("a module installed in py/venv is held")
	}
}

// TestBuild_TheHeaderNamesTheFirstLeftOutDirs (L2, the bound 20): with n
// environments the result names all n and the header counts n and names
// the first graph.MaxRefusedNamed by path. Break: name every one in the
// header; count only the named.
func TestBuild_TheHeaderNamesTheFirstLeftOutDirs(t *testing.T) {
	for _, n := range []int{graph.MaxRefusedNamed, graph.MaxRefusedNamed + 1} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			files := map[string]string{"py/app/a.py": "def run():\n    return 1\n"}
			for i := range n {
				files[fmt.Sprintf("py/e%02d/pyvenv.cfg", i)] = "home = /usr/bin\n"
				files[fmt.Sprintf("py/e%02d/m.py", i)] = "x = 1\n"
			}
			r, err := Build(wireRoot(t, files), "c1", pyUnit)
			if err != nil {
				t.Fatal(err)
			}
			last := fmt.Sprintf("py/e%02d", graph.MaxRefusedNamed-1)
			h := r.Header
			if len(r.Left) != n || h.LeftOut != n || len(h.LeftOutDirs) != graph.MaxRefusedNamed || h.LeftOutDirs[len(h.LeftOutDirs)-1].Path != last {
				t.Fatalf("left %d; header %d, named %d %+v; want %d, %d, the first %d ending %s",
					len(r.Left), h.LeftOut, len(h.LeftOutDirs), h.LeftOutDirs, n, n, graph.MaxRefusedNamed, last)
			}
			if r.Files != 1 {
				t.Errorf("files = %d; want 1", r.Files)
			}
		})
	}
}

// TestBuild_ASkippedDirectoryIsCountedByKind (L2, e4): the walk counts the
// directories it skips by name, by kind: dot directories but not .git,
// .rashomon or .pgraph at any depth; node_modules; __pycache__; never a
// directory inside a skipped one. An environment under a dot name (.venv)
// is in the dot count and is not named. Break: count .git, .rashomon or
// .pgraph; count inside a skipped directory; name .venv.
func TestBuild_ASkippedDirectoryIsCountedByKind(t *testing.T) {
	root := wireRoot(t, map[string]string{
		".git/HEAD":                            "ref: refs/heads/main\n",
		".git/config":                          "[core]\n\tbare = false\n",
		".rashomon/graph.json":                 `{"units":[{"path":"py","mod_root":"py","modules":["app"],"lang":"py"}]}`,
		".pgraph/pgraph.log":                   "",
		"sub/.git/HEAD":                        "ref: refs/heads/main\n",
		"svc/.pgraph/pgraph.log":               "",
		".github/workflows/ci.yml":             "on: push\n",
		"py/.cache/c.py":                       "c = 1\n",
		".venv/pyvenv.cfg":                     "home = /usr/bin\n",
		".venv/lib/x.py":                       "x = 1\n",
		"py/.hidden/.deeper/x.py":              "x = 1\n",
		"node_modules/m/index.js":              "",
		"node_modules/m/node_modules/k/i.js":   "",
		"web/node_modules/n/i.js":              "",
		"py/app/__pycache__/a.cpython-312.pyc": "",
		"py/app/a.py":                          "def run():\n    return 1\n",
	})
	r, err := Build(root, "c1", pyUnit)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Skipped{Dot: 4, NodeModules: 2, PyCache: 1}); r.Skipped != want {
		t.Errorf("skipped %+v; want %+v (.github, py/.cache, .venv, py/.hidden; two node_modules; one __pycache__)", r.Skipped, want)
	}
	if len(r.Left) != 0 || r.Header.LeftOut != 0 {
		t.Errorf("left %+v, header %d; want none: .venv is a dot directory", r.Left, r.Header.LeftOut)
	}
	if r.Files != 1 {
		t.Errorf("files = %d; want 1", r.Files)
	}
}

// TestBuild_AGoOnlyIndexAsksNoLanguageRule (L2 control): an index that
// configures no language with a rule (Go only) leaves out no directory,
// even one holding pyvenv.cfg, and its result and header are those of the
// same tree without it. Break: ask every registered resolver, not the
// configured ones.
func TestBuild_AGoOnlyIndexAsksNoLanguageRule(t *testing.T) {
	goOnly := Config{Units: []Unit{{Path: ".", Lang: gopg.Lang}}}
	root := shapes(t)
	before, err := Build(root, "c1", goOnly)
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "shape", "pyvenv.cfg"), "home = /usr/bin\n")
	after, err := Build(root, "c1", goOnly)
	if err != nil {
		t.Fatal(err)
	}
	rev := graph.RevCommit("c1")
	if after.Files != before.Files || len(after.Store.Nodes(rev)) != len(before.Store.Nodes(rev)) || len(after.Left) != 0 {
		t.Errorf("Go only with shape/pyvenv.cfg: files %d, nodes %d, left %+v; want files %d, nodes %d, none left",
			after.Files, len(after.Store.Nodes(rev)), after.Left, before.Files, len(before.Store.Nodes(rev)))
	}
	if !reflect.DeepEqual(after.Header, before.Header) {
		t.Errorf("header %+v; want %+v", after.Header, before.Header)
	}
	if b, _ := json.Marshal(after.Header); strings.Contains(string(b), "left_out") {
		t.Errorf("header line %s holds left_out", b)
	}
}

// TestInfer_AnEnvironmentIsLeftOutAndNamed (L2): the inference walk leaves
// a directory holding pyvenv.cfg out and names it with the rule's reason,
// so neither its installed packages nor an editable checkout under its src
// becomes a unit or a left-out line of its own; .venv is skipped by its dot
// and named by nothing. Break: walk the environment (its files then read
// as a directory of source and a nested project); name .venv.
func TestInfer_AnEnvironmentIsLeftOutAndNamed(t *testing.T) {
	inf, err := Infer(wireRoot(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"app\"\n",
		"app/__init__.py": "",
		"venv/pyvenv.cfg": "home = /usr/bin\n",
		"venv/lib/python3.12/site-packages/dep/__init__.py":   "",
		"venv/src/edit/pyproject.toml":                        "[project]\nname = \"edit\"\n",
		"venv/src/edit/edit/__init__.py":                      "",
		".venv/pyvenv.cfg":                                    "home = /usr/bin\n",
		".venv/lib/python3.12/site-packages/dep2/__init__.py": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := declaredRows(inf.Config.Units), []string{"py|app|.|app"}; !slices.Equal(got, want) {
		t.Errorf("units %v; want %v", got, want)
	}
	if want := []lang.Left{{Path: "venv", Why: envWhy}}; !reflect.DeepEqual(inf.Left, want) {
		t.Errorf("left out %+v; want %+v", inf.Left, want)
	}
}

// TestUnitOf_KnownLimit_APathUnderANamedEnvironmentStillMapsToItsUnit (L2):
// a known limit, pinned on purpose. UnitOf reads paths alone, so a path
// under a directory holding pyvenv.cfg inside a Python unit still maps to
// that unit, while Build's walk leaves the directory out and holds no file
// under it. The two disagree on such paths until the card that makes the
// Stop ask the language's rule; that card flips this test on purpose.
// Break: a UnitOf that reads pyvenv.cfg, which this card does not make.
func TestUnitOf_KnownLimit_APathUnderANamedEnvironmentStillMapsToItsUnit(t *testing.T) {
	root := wireRoot(t, map[string]string{
		"py/app/__init__.py": "",
		"py/venv/pyvenv.cfg": "home = /usr/bin\n",
		"py/venv/lib/m.py":   "x = 1\n",
	})
	unitOf, err := UnitOf(pyUnit)
	if err != nil {
		t.Fatal(err)
	}
	want := graph.UnitID(pylite.Lang, "py")
	if got, ok := unitOf(root, "py/venv/lib/m.py"); got != want || !ok {
		t.Errorf("UnitOf(py/venv/lib/m.py) = %q, %v; want %q, true: the known limit moved", got, ok, want)
	}
}
