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

// leftLines is each left-out path and its reason, as graph index prints it.
func leftLines(ls []lang.Left) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Path+": "+l.Why)
	}
	return out
}

// TestSETUP1_PY1_ASrcLayoutPackageIsAUnitAndNestedProjectsAreLeftOut: in
// click's layout the one package under src is the one unit (mod_root src,
// modules [click]); the projects under examples, one a package of its own,
// are left out and named, and so are tests and docs, which hold .py files
// outside the modules' root.
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
	if want := []string{"docs", "examples/aliases", "examples/complex", "tests"}; !slices.Equal(leftPaths(got.Left), want) {
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

// TestSETUP1_PY4_InASrcLayoutWhatLiesOutsideSrcIsNamed: when a project's
// modules start at its src directory, each directory at the project's
// directory holding .py files, at any depth, and each .py file there but the
// setup.py manifest, is left out and named once, as the flat layout names
// them; a directory of no .py file is not named, and a project inside this
// one is named as that, not as this project's files.
func TestSETUP1_PY4_InASrcLayoutWhatLiesOutsideSrcIsNamed(t *testing.T) {
	files := []string{
		"docs/index.rst",
		"examples/demo/demo.py",
		"examples/demo/pyproject.toml",
		"noxfile.py",
		"pyproject.toml",
		"scripts/tool/run.py",
		"setup.py",
		"src/pkg/__init__.py",
		"tests/test_a.py",
		"tests/unit/test_b.py",
	}
	got := Resolver{}.Infer(files)
	if want := []unitRow{{"src/pkg", "src", "pkg", "pyproject.toml"}}; !slices.Equal(rows(got.Units), want) {
		t.Errorf("units %+v; want %+v", rows(got.Units), want)
	}
	want := []string{
		"examples/demo: a project inside another project",
		"noxfile.py: source outside the modules' root (a .py file, and the project's modules start at src)",
		"scripts: source outside the modules' root (.py files in it, and the project's modules start at src)",
		"tests: source outside the modules' root (.py files in it, and the project's modules start at src)",
	}
	if l := leftLines(got.Left); !slices.Equal(l, want) {
		t.Errorf("left out:\n%s\nwant:\n%s", strings.Join(l, "\n"), strings.Join(want, "\n"))
	}
}

// TestSETUP1_PY5_EachReasonIsAKindThenWhatWasChecked: the reason for every
// path left out begins with one of package lang's kinds, which name no
// language, and then, in parentheses where it adds something, what the rule
// checked in Python's terms. A directory of .py files with no __init__.py
// is named for that alone, not as a namespace package: a manifest may place
// a package there, and the rule reads no manifest's settings.
func TestSETUP1_PY5_EachReasonIsAKindThenWhatWasChecked(t *testing.T) {
	files := []string{
		"app/conftest.py",
		"app/pyproject.toml",
		"app/src/nsp/b.py",
		"app/src/pkg/__init__.py",
		"app/src/run.py",
		"app/tests/test_x.py",
		"flat/inner/pyproject.toml",
		"flat/nspkg/sub/a.py",
		"flat/pkg/__init__.py",
		"flat/pyproject.toml",
		"flat/tool.py",
	}
	got := Resolver{}.Infer(files)
	if want := []unitRow{{"app/src/pkg", "app/src", "pkg", "app/pyproject.toml"}, {"flat/pkg", "flat", "pkg", "flat/pyproject.toml"}}; !slices.Equal(rows(got.Units), want) {
		t.Errorf("units %+v; want %+v", rows(got.Units), want)
	}
	want := []string{
		"app/conftest.py: source outside the modules' root (a .py file, and the project's modules start at src)",
		"app/src/nsp: a directory of source that is not a package (.py files under it and no __init__.py directly in it)",
		"app/src/run.py: a module file at the modules' root, not in a package",
		"app/tests: source outside the modules' root (.py files in it, and the project's modules start at src)",
		"flat/inner: a project inside another project",
		"flat/nspkg: a directory of source that is not a package (.py files under it and no __init__.py directly in it)",
		"flat/tool.py: a module file at the modules' root, not in a package",
	}
	if l := leftLines(got.Left); !slices.Equal(l, want) {
		t.Errorf("left out:\n%s\nwant:\n%s", strings.Join(l, "\n"), strings.Join(want, "\n"))
	}
	kinds := []string{lang.LeftNested, lang.LeftLooseModule, lang.LeftNotPackage, lang.LeftOutsideRoot}
	words := append([]string{".py", "__init__", "python"}, Resolver{}.Manifests()...)
	for _, l := range got.Left {
		kind, _, _ := strings.Cut(l.Why, " (")
		if !slices.Contains(kinds, kind) {
			t.Errorf("%s: reason %q begins with none of package lang's kinds", l.Path, l.Why)
		}
		for _, w := range words {
			if strings.Contains(strings.ToLower(kind), w) || slices.Contains(strings.Fields(kind), "py") {
				t.Errorf("%s: the kind %q names Python (%q); the language's words go in the parentheses", l.Path, kind, w)
				break
			}
		}
	}
}
