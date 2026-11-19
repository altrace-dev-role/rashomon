package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// declaredRows is units as a configuration declares them, one string each.
func declaredRows(us []Unit) []string {
	var out []string
	for _, u := range us {
		out = append(out, fmt.Sprintf("%s|%s|%s|%s", u.Lang, u.Path, u.ModRoot, strings.Join(u.Modules, ",")))
	}
	return out
}

// inferredLeft is the paths Infer left out.
func inferredLeft(inf Inferred) []string {
	var out []string
	for _, l := range inf.Left {
		out = append(out, l.Path)
	}
	return out
}

// clickFiles is the layout of a copy of click (fe307c1e): its project
// manifest, its one package under src, its tests and docs, and the ten
// example projects under examples, one of them a package of its own.
func clickFiles() map[string]string {
	files := map[string]string{}
	files["pyproject.toml"] = "[project]\nname = \"click\"\n"
	files["docs/conf.py"] = "project = 'click'\n"
	files["examples/README"] = "examples\n"
	files["examples/complex/complex/__init__.py"] = ""
	files["examples/complex/complex/commands/__init__.py"] = ""
	files["tests/test_utils/__init__.py"] = ""
	files["tests/test_types/__init__.py"] = ""
	files["tests/test_exceptions/__init__.py"] = ""
	files["src/click/py.typed"] = ""
	for _, m := range []string{"__init__", "_compat", "_termui_impl", "_textwrap", "_utils", "_winconsole", "core",
		"decorators", "exceptions", "formatting", "globals", "parser", "shell_completion", "termui", "testing", "types", "utils"} {
		files["src/click/"+m+".py"] = "x = 1\n"
	}
	for _, ex := range []string{"aliases", "colors", "completion", "complex", "imagepipe", "inout", "naval", "repo", "termui", "validation"} {
		files["examples/"+ex+"/pyproject.toml"] = "[project]\nname = \"" + ex + "\"\n"
	}
	return files
}

// TestSETUP1_R1_AClickCopyGivesItsRecordTruth (R1): click's layout with no
// configuration infers one unit, src/click with mod_root src owning the
// module click. The truth is not this rule's: the installed click-8.4.1
// wheel's RECORD lists one top-level name, click. The ten example projects
// are left out and named, and so are docs and tests, which hold .py files
// outside src; the file decodes to the configuration.
func TestSETUP1_R1_AClickCopyGivesItsRecordTruth(t *testing.T) {
	inf, err := Infer(wireRoot(t, clickFiles()))
	if err != nil {
		t.Fatalf("Infer = %v; want click's unit", err)
	}
	if got, want := declaredRows(inf.Config.Units), []string{"py|src/click|src|click"}; !slices.Equal(got, want) {
		t.Errorf("units %v; want %v (RECORD: one top-level name, click)", got, want)
	}
	want := []string{"docs"}
	for _, ex := range []string{"aliases", "colors", "completion", "complex", "imagepipe", "inout", "naval", "repo", "termui", "validation"} {
		want = append(want, "examples/"+ex)
	}
	want = append(want, "tests")
	if got := inferredLeft(inf); !slices.Equal(got, want) {
		t.Errorf("left out %v; want %v", got, want)
	}
	var back Config
	if err := json.Unmarshal(inf.Bytes, &back); err != nil || !slices.Equal(declaredRows(back.Units), declaredRows(inf.Config.Units)) {
		t.Errorf("the file %q decodes to %+v, %v; want the configuration", inf.Bytes, back, err)
	}
	if !strings.Contains(string(inf.Bytes), "\"service_map\": {},\n  \"knowledge\": []\n}\n") {
		t.Errorf("the file %q; want an empty service map and knowledge list, as a hand-written file has", inf.Bytes)
	}
}

// moduleRoot is the root of this repository's module.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Join(filepath.Dir(f), "..", "..", "..")
}

// TestSETUP1_R2_ThisTreeGivesItsCommittedFile (R2): inferred over this
// repository, the file is byte for byte the .rashomon/graph.json a person
// wrote and committed (units . and pgraph): the nine go.mod under testdata
// are not units, and test/fixtures/lang/py-unit, which holds a
// configuration of its own, is another index's root, left out and named.
func TestSETUP1_R2_ThisTreeGivesItsCommittedFile(t *testing.T) {
	root := moduleRoot(t)
	inf, err := Infer(root)
	if err != nil {
		t.Fatalf("Infer over this tree = %v", err)
	}
	want, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ConfigFile)))
	if err != nil {
		t.Fatal(err)
	}
	if string(inf.Bytes) != string(want) {
		t.Errorf("inferred file:\n%s\nwant the committed file:\n%s", inf.Bytes, want)
	}
	if !slices.Contains(inferredLeft(inf), "test/fixtures/lang/py-unit") {
		t.Errorf("left out %v; want test/fixtures/lang/py-unit named", inferredLeft(inf))
	}
}

// manifestNames are the manifests ErrNoConfig's error names.
var manifestNames = []string{"go.mod", "pyproject.toml", "setup.cfg", "setup.py"}

// TestSETUP1_R5_NoUnitIsErrNoConfig (R5): a tree with no manifest, and one
// whose only project has no package, infer no unit: the error matches
// ErrNoConfig and names every manifest looked for, and what was left out
// is still returned, for the person to read.
func TestSETUP1_R5_NoUnitIsErrNoConfig(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		left  []string
	}{
		{"no manifest", map[string]string{"README": "nothing here\n"}, nil},
		{"a project with no package", map[string]string{"pyproject.toml": "", "tool.py": "x = 1\n"}, []string{"tool.py"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			inf, err := Infer(wireRoot(t, c.files))
			if !errors.Is(err, ErrNoConfig) {
				t.Fatalf("Infer = %v; want ErrNoConfig", err)
			}
			for _, m := range manifestNames {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error %q; want it to name %s", err, m)
				}
			}
			if got := inferredLeft(inf); !slices.Equal(got, c.left) {
				t.Errorf("left out %v; want %v", got, c.left)
			}
		})
	}
}

// TestSETUP1_IX1_AConfigurationBelowTheRootIsAnotherIndex: a directory
// below the root holding a configuration of its own is not walked; its
// module is no unit of this root's, and it is named as left out.
func TestSETUP1_IX1_AConfigurationBelowTheRootIsAnotherIndex(t *testing.T) {
	inf, err := Infer(wireRoot(t, map[string]string{
		"go.mod":                     "module example.com/outer\n",
		"a.go":                       "package outer\n",
		"inner/go.mod":               "module example.com/inner\n",
		"inner/.rashomon/graph.json": `{"units":[{"path":".","lang":"go"}]}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := declaredRows(inf.Config.Units), []string{"go|.|.|"}; !slices.Equal(got, want) {
		t.Errorf("units %v; want %v", got, want)
	}
	if got := inferredLeft(inf); !slices.Equal(got, []string{"inner"}) {
		t.Errorf("left out %v; want [inner]", got)
	}
}

// TestSETUP1_R8_AFileOverTheCapIsRefused (R8): a configuration whose file
// would be over MaxConfigBytes is refused by an error matching
// regfile.ErrOverCap that names the cap; no bytes are returned. Two hundred
// packages with 190-byte names infer about 90 KB in this file's shape and
// 88 KB as compact JSON, so any shape is over the 64 KiB cap.
func TestSETUP1_R8_AFileOverTheCapIsRefused(t *testing.T) {
	files := map[string]string{"pyproject.toml": ""}
	for i := range 200 {
		files[fmt.Sprintf("src/p%03d_%s/__init__.py", i, strings.Repeat("x", 185))] = ""
	}
	inf, err := Infer(wireRoot(t, files))
	if !errors.Is(err, regfile.ErrOverCap) || !strings.Contains(err.Error(), strconv.Itoa(MaxConfigBytes)) {
		t.Fatalf("Infer = %v; want the cap's refusal naming %d", err, MaxConfigBytes)
	}
	if len(inf.Bytes) != 0 {
		t.Errorf("Infer returned %d bytes with its refusal; want none", len(inf.Bytes))
	}
}

// treeInOrder is a directory holding files, made in the order of rels.
func treeInOrder(t *testing.T, files map[string]string, rels []string) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range rels {
		put(t, filepath.Join(root, filepath.FromSlash(rel)), files[rel])
	}
	return root
}

// TestSETUP1_M2_InferenceIsByteStable (pb40 M2): two fresh copies of one
// tree, and a copy whose files were made in the other order, give the same
// bytes, and a second Infer of one copy gives them again: neither map
// order nor directory order reaches the file.
func TestSETUP1_M2_InferenceIsByteStable(t *testing.T) {
	files := clickFiles()
	files["go.mod"] = "module example.com/click\n"
	files["tools/go.mod"] = "module example.com/tools\n"
	files["lib/setup.cfg"] = ""
	files["lib/b/__init__.py"] = ""
	files["lib/a/__init__.py"] = ""
	var rels []string
	for rel := range files {
		rels = append(rels, rel)
	}
	slices.Sort(rels)
	reversed := slices.Clone(rels)
	slices.Reverse(reversed)
	var got [][]byte
	for _, root := range []string{treeInOrder(t, files, rels), treeInOrder(t, files, rels), treeInOrder(t, files, reversed)} {
		for range 2 {
			inf, err := Infer(root)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, inf.Bytes)
		}
	}
	for i, b := range got[1:] {
		if string(b) != string(got[0]) {
			t.Errorf("Infer %d gave:\n%s\nwant the first's bytes:\n%s", i+2, b, got[0])
		}
	}
}

// TestSETUP1_L2_AManifestLinkedOutsideTheRootIsNotAUnit (pb40 L2): a
// manifest that is a link to a file outside the root is neither read nor
// taken as a project, so the package beside it makes no unit.
func TestSETUP1_L2_AManifestLinkedOutsideTheRootIsNotAUnit(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "pyproject.toml")
	put(t, outside, "[project]\nname = \"elsewhere\"\n")
	root := wireRoot(t, map[string]string{"src/pkg/__init__.py": ""})
	if err := os.Symlink(outside, filepath.Join(root, "pyproject.toml")); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
	inf, err := Infer(root)
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("Infer = %v, units %v; want ErrNoConfig, the linked manifest not taken", err, declaredRows(inf.Config.Units))
	}
}
