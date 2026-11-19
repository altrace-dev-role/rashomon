package pylite

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// unitRow is an inferred unit as a configuration declares it.
type unitRow struct {
	path, modRoot, modules, from string
}

func rows(us []lang.Unit) []unitRow {
	var out []unitRow
	for _, u := range us {
		out = append(out, unitRow{u.Path, u.ModRoot, strings.Join(u.Modules, ","), u.From})
	}
	return out
}

func leftPaths(ls []lang.Left) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Path)
		if l.Why == "" {
			out = append(out, "(no reason)")
		}
	}
	return out
}

// TestSETUP1_PY1_ASrcLayoutPackageIsAUnitAndNestedProjectsAreLeftOut: in
// click's layout the one package under src is the one unit (mod_root src,
// modules [click]); the projects under examples, one a package of its own,
// are left out and named; tests and docs, outside the modules' root, are
// neither.
func TestSETUP1_PY1_ASrcLayoutPackageIsAUnitAndNestedProjectsAreLeftOut(t *testing.T) {
	files := []string{
		"docs/conf.py",
		"examples/aliases/aliases.py",
		"examples/aliases/pyproject.toml",
		"examples/complex/complex/__init__.py",
		"examples/complex/pyproject.toml",
		"pyproject.toml",
		"src/click/__init__.py",
		"src/click/core.py",
		"src/click/py.typed",
		"tests/conftest.py",
		"tests/test_utils/__init__.py",
	}
	got := Resolver{}.Infer(files)
	if want := []unitRow{{"src/click", "src", "click", "pyproject.toml"}}; !slices.Equal(rows(got.Units), want) {
		t.Errorf("units %+v; want %+v", rows(got.Units), want)
	}
	if want := []string{"examples/aliases", "examples/complex"}; !slices.Equal(leftPaths(got.Left), want) {
		t.Errorf("left out %v; want %v", leftPaths(got.Left), want)
	}
}

// TestSETUP1_PY2_AFlatLayoutPackageIsAUnit: with no src package the modules
// start at the project's directory, "." at the root; each package there is
// a unit, the manifest is not a module, and a project in a directory of its
// own beside it is a project too.
func TestSETUP1_PY2_AFlatLayoutPackageIsAUnit(t *testing.T) {
	files := []string{
		"other/__init__.py",
		"pkg/__init__.py",
		"pkg/a.py",
		"setup.cfg",
		"setup.py",
		"src/notes.txt",
	}
	got := Resolver{}.Infer(files)
	want := []unitRow{{"other", ".", "other", "setup.cfg"}, {"pkg", ".", "pkg", "setup.cfg"}}
	if !slices.Equal(rows(got.Units), want) {
		t.Errorf("units %+v; want %+v", rows(got.Units), want)
	}
	if len(got.Left) != 0 {
		t.Errorf("left out %v; want nothing", leftPaths(got.Left))
	}
}

// TestSETUP1_PY3_AModuleFileAndANamespacePackageAreNamed: at the modules'
// root a top-level module file and a directory of .py files with no
// __init__.py are left out and named; a directory of no .py file is
// neither; two sibling projects each take their own src.
func TestSETUP1_PY3_AModuleFileAndANamespacePackageAreNamed(t *testing.T) {
	files := []string{
		"app/pyproject.toml",
		"app/src/data/x.txt",
		"app/src/nspkg/a.py",
		"app/src/pkg/__init__.py",
		"app/src/tool.py",
		"svc/pyproject.toml",
		"svc/src/svc/__init__.py",
	}
	got := Resolver{}.Infer(files)
	want := []unitRow{{"app/src/pkg", "app/src", "pkg", "app/pyproject.toml"}, {"svc/src/svc", "svc/src", "svc", "svc/pyproject.toml"}}
	if !slices.Equal(rows(got.Units), want) {
		t.Errorf("units %+v; want %+v", rows(got.Units), want)
	}
	if want := []string{"app/src/nspkg", "app/src/tool.py"}; !slices.Equal(leftPaths(got.Left), want) {
		t.Errorf("left out %v; want %v", leftPaths(got.Left), want)
	}
}
