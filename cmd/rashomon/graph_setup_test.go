package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
)

// setupApp is a Python project with one package under src, no
// configuration.
var setupApp = map[string]string{
	"pyproject.toml":      "[project]\nname = \"app\"\n",
	"src/app/__init__.py": "",
	"src/app/core.py":     "def run(x):\n    return x\n",
}

// setupConfig is a configuration a person wrote for setupApp.
const setupConfig = "{\"units\": [{\"path\": \"src/app\", \"mod_root\": \"src\", \"modules\": [\"app\"], \"lang\": \"py\"}]}\n"

// setupTree is a directory holding the files of each map, each a slash path
// and its text.
func setupTree(t *testing.T, sets ...map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for _, files := range sets {
		for rel, text := range files {
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

// setupRun is graph index over dir: what it printed and its error.
func setupRun(dir string) (string, error) {
	var out strings.Builder
	err := graphIndex([]string{dir}, &out)
	return out.String(), err
}

// setupPath is the configuration's path under dir.
func setupPath(dir string) string { return filepath.Join(dir, filepath.FromSlash(index.ConfigFile)) }

// setupList is the names dir holds, nil for none.
func setupList(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// setupSame fails the test unless p is the file it was: bytes, mode,
// inode and modification time.
func setupSame(t *testing.T, p string, b []byte, fi os.FileInfo) {
	t.Helper()
	got, err := os.ReadFile(p)
	if err != nil || string(got) != string(b) {
		t.Errorf("%s holds %q, %v; want %q", p, got, err, b)
	}
	now, err := os.Stat(p)
	if err != nil || !os.SameFile(fi, now) || now.Mode() != fi.Mode() || !now.ModTime().Equal(fi.ModTime()) {
		t.Errorf("%s is not the same file with the same mode and time after the run (%v)", p, err)
	}
}

// setupWrote is whether out holds a line about a write or an inference.
func setupWrote(out string) bool {
	return strings.Contains(out, "wrote ") || strings.Contains(out, "inferred unit")
}

// TestSETUP1_R1c_NoConfigIsInferredWrittenAndIndexed (R1, M3): with no
// configuration, graph index names the unit it inferred and the manifest it
// came from, writes the file, says so, and indexes it. The directory and
// the file it makes have the modes a create with 0755 and 0644 gives under
// this process's umask.
func TestSETUP1_R1c_NoConfigIsInferredWrittenAndIndexed(t *testing.T) {
	dir := setupTree(t, setupApp)
	out, err := setupRun(dir)
	if err != nil {
		t.Fatalf("graph index = %v; want the configuration inferred and indexed\n%s", err, out)
	}
	for _, want := range []string{"inferred unit src/app (py) from pyproject.toml: ", "wrote .rashomon/graph.json: 1 unit(s)", "indexed 2 file(s)"} {
		if strings.Count(out, want) != 1 {
			t.Errorf("graph index printed:\n%s\nwant once: %s", out, want)
		}
	}
	c, err := index.ReadConfig(dir)
	if err != nil || len(c.Units) != 1 || c.Units[0].Path != "src/app" || c.Units[0].ModRoot != "src" || !slices.Equal(c.Units[0].Modules, []string{"app"}) {
		t.Errorf("the file reads %+v, %v; want the unit src/app", c, err)
	}
	if _, _, err := graph.LoadFile(dir); err != nil {
		t.Errorf("no index after the run: %v", err)
	}
	ref := t.TempDir()
	if err := os.Mkdir(filepath.Join(ref, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ref, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{filepath.Join(dir, ".rashomon"), filepath.Join(ref, "d")}, {setupPath(dir), filepath.Join(ref, "f")}} {
		got, err1 := os.Stat(pair[0])
		want, err2 := os.Stat(pair[1])
		if err1 != nil || err2 != nil {
			t.Errorf("stat %s, %s: %v, %v", pair[0], pair[1], err1, err2)
		} else if got.Mode() != want.Mode() {
			t.Errorf("%s mode %v; want %v, as a plain create makes it", pair[0], got.Mode(), want.Mode())
		}
	}
}

// TestSETUP1_R3_AFileAtThePathIsLeftAsItIs (R3): a configuration a person
// wrote is read as it is; its bytes, mode, inode and time are unchanged and
// no line speaks of a write.
func TestSETUP1_R3_AFileAtThePathIsLeftAsItIs(t *testing.T) {
	dir := setupTree(t, setupApp, map[string]string{index.ConfigFile: setupConfig})
	if err := os.Chmod(setupPath(dir), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(setupPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	out, err := setupRun(dir)
	if err != nil || setupWrote(out) {
		t.Errorf("graph index = %v, printed:\n%s\nwant the file indexed with no write", err, out)
	}
	setupSame(t, setupPath(dir), []byte(setupConfig), fi)
}

// TestSETUP1_R3b_ADirectoryAtThePathIsLeftAsItIs (R3): a directory at the
// path fails the run and holds what it held, the same directory.
func TestSETUP1_R3b_ADirectoryAtThePathIsLeftAsItIs(t *testing.T) {
	dir := setupTree(t, setupApp, map[string]string{index.ConfigFile + "/inside": "x"})
	fi, err := os.Stat(setupPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	out, err := setupRun(dir)
	if err == nil || setupWrote(out) {
		t.Errorf("graph index = %v, printed:\n%s\nwant a failure and no write", err, out)
	}
	now, err := os.Stat(setupPath(dir))
	if err != nil || !now.IsDir() || !os.SameFile(fi, now) || !slices.Equal(setupList(t, setupPath(dir)), []string{"inside"}) {
		t.Errorf("the directory at the path changed (%v)", err)
	}
}

// TestSETUP1_R4_ALinkAtThePathIsLeftAsItIs (R4, pb40 L2): a link at the path,
// to a configuration in the tree or outside it, is read through as today; a
// dangling one fails the run; a .rashomon that is a link to another
// directory of the tree fails it too. Each link reads as it did, each
// target holds what it held, and nothing is made where a link points.
func TestSETUP1_R4_ALinkAtThePathIsLeftAsItIs(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(outside, []byte(setupConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, at, target, made string // made: the target's path under dir, when the test makes it
		fails                  bool
		absent                 string // a path under dir nothing may be made at
	}{
		{"to a file in the tree", index.ConfigFile, "real.json", ".rashomon/real.json", false, ""},
		{"to a file outside the tree", index.ConfigFile, outside, "", false, ""},
		{"to nothing", index.ConfigFile, "missing.json", "", true, ".rashomon/missing.json"},
		{".rashomon to another directory", ".rashomon", "other", "", true, "other/graph.json"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := setupTree(t, setupApp)
			at := filepath.Join(dir, filepath.FromSlash(c.at))
			if err := os.MkdirAll(filepath.Join(dir, "other"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
				t.Fatal(err)
			}
			if c.made != "" {
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(c.made)), []byte(setupConfig), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(c.target, at); err != nil {
				t.Skipf("no symlink here: %v", err)
			}
			out, err := setupRun(dir)
			if (err != nil) != c.fails || strings.Contains(out, "wrote ") {
				t.Errorf("graph index = %v, printed:\n%s\nwant failure %v and no write", err, out, c.fails)
			}
			if got, err := os.Readlink(at); err != nil || got != c.target {
				t.Errorf("the link reads %q, %v; want %q", got, err, c.target)
			}
			for _, p := range []string{outside, filepath.Join(dir, filepath.FromSlash(c.made))} {
				if p == dir {
					continue
				}
				if got, err := os.ReadFile(p); err != nil || string(got) != setupConfig {
					t.Errorf("%s holds %q, %v; want what it held", p, got, err)
				}
			}
			if c.absent != "" {
				if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(c.absent))); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Lstat %s = %v; want nothing made there", c.absent, err)
				}
			}
		})
	}
}

// TestSETUP1_R5_AnEmptyTreeIsErrNoConfigAndWritesNothing (R5, M3): a tree
// with no manifest fails with ErrNoConfig naming every manifest looked for,
// and the root lists what it listed before: no .rashomon is left.
func TestSETUP1_R5_AnEmptyTreeIsErrNoConfigAndWritesNothing(t *testing.T) {
	dir := setupTree(t, map[string]string{"README": "nothing here\n"})
	out, err := setupRun(dir)
	if !errors.Is(err, index.ErrNoConfig) {
		t.Fatalf("graph index = %v; want ErrNoConfig\n%s", err, out)
	}
	for _, m := range []string{"go.mod", "pyproject.toml", "setup.cfg", "setup.py"} {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("error %q; want it to name %s", err, m)
		}
	}
	if got := setupList(t, dir); !slices.Equal(got, []string{"README"}) {
		t.Errorf("the root lists %v; want [README]", got)
	}
}

// TestSETUP1_R6_NestedProjectsAreNamedUpToTwenty (R6, pb38 C): projects
// inside the project are left out, each named on a line of its own up to
// twenty, and past twenty one line counts the rest; the file holds the one
// unit whatever their number.
func TestSETUP1_R6_NestedProjectsAreNamedUpToTwenty(t *testing.T) {
	for _, n := range []int{0, 1, 20, 21} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			nested := map[string]string{}
			for i := range n {
				nested[fmt.Sprintf("examples/ex%02d/pyproject.toml", i)] = ""
			}
			dir := setupTree(t, setupApp, nested)
			out, err := setupRun(dir)
			if err != nil {
				t.Fatalf("graph index = %v\n%s", err, out)
			}
			if got := strings.Count(out, "left out examples/ex"); got != min(n, 20) {
				t.Errorf("%d named line(s) in:\n%s\nwant %d", got, out, min(n, 20))
			}
			more := strings.Contains(out, "left out 1 more path(s)\n")
			if more != (n == 21) || (n != 21 && strings.Contains(out, "more path(s)")) {
				t.Errorf("printed:\n%s\nwant the count line only past twenty", out)
			}
			if c, err := index.ReadConfig(dir); err != nil || len(c.Units) != 1 {
				t.Errorf("the file reads %+v, %v; want one unit", c, err)
			}
		})
	}
}

// TestSETUP1_R7_ASecondRunWritesNothing (R7): the second run reads the file
// the first wrote; its bytes, mode, inode and time are unchanged and the
// second run prints no line about a write or an inference.
func TestSETUP1_R7_ASecondRunWritesNothing(t *testing.T) {
	dir := setupTree(t, setupApp)
	if out, err := setupRun(dir); err != nil {
		t.Fatalf("first graph index = %v\n%s", err, out)
	}
	b, err := os.ReadFile(setupPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(setupPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	out, err := setupRun(dir)
	if err != nil || setupWrote(out) {
		t.Errorf("second graph index = %v, printed:\n%s\nwant no write and no inference", err, out)
	}
	setupSame(t, setupPath(dir), b, fi)
}

// TestSETUP1_R8_AnInferredFileOverTheCapIsNotWritten (R8, pb40 L1): an
// inferred file over index.MaxConfigBytes fails the run with an error
// naming the cap, and the root lists what it listed before.
func TestSETUP1_R8_AnInferredFileOverTheCapIsNotWritten(t *testing.T) {
	files := map[string]string{"pyproject.toml": ""}
	for i := range 200 { // about 90 KB inferred: over the cap in any shape
		files[fmt.Sprintf("src/p%03d_%s/__init__.py", i, strings.Repeat("x", 185))] = ""
	}
	dir := setupTree(t, files)
	before := setupList(t, dir)
	out, err := setupRun(dir)
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(index.MaxConfigBytes)) {
		t.Fatalf("graph index = %v; want an error naming %d\n%s", err, index.MaxConfigBytes, out)
	}
	if got := setupList(t, dir); !slices.Equal(got, before) || strings.Contains(out, "wrote ") {
		t.Errorf("the root lists %v, printed:\n%s\nwant %v and no write", got, out, before)
	}
}

// TestSETUP1_SH1_SetupLinesShowPathsAsShownDoes (pb38): a unit's path and a
// left-out path holding a rune shown refuses print as Go quoted strings,
// so no file name writes a line of its own or turns its line around.
func TestSETUP1_SH1_SetupLinesShowPathsAsShownDoes(t *testing.T) {
	odd := []string{"src/b\u202ec", "examples/ex\u202eone"}
	files := map[string]string{"src/b\u202ec/__init__.py": "", "examples/ex\u202eone/pyproject.toml": ""}
	nl := "examples/ex\ntwo"
	dir := setupTree(t, setupApp, files)
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(nl)), 0o755); err == nil {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(nl), "pyproject.toml"), nil, 0o644); err == nil {
			odd = append(odd, nl)
		}
	}
	out, err := setupRun(dir)
	if err != nil {
		t.Fatalf("graph index = %v\n%s", err, out)
	}
	for _, p := range odd {
		if !strings.Contains(out, " "+strconv.Quote(p)+" ") && !strings.Contains(out, " "+strconv.Quote(p)+":") {
			t.Errorf("printed:\n%s\nwant %s quoted", out, strconv.Quote(p))
		}
	}
	if strings.ContainsRune(out, '\u202e') || strings.Contains(out, "ex\ntwo") {
		t.Errorf("printed:\n%q\nwant no path printed raw", out)
	}
}

// TestSETUP1_M1b_ARacingWriterIsReadNotOverwritten (pb40 M1b, a1): a file put
// on the name between the read that found none and the create is the
// racer's: its bytes and inode are unchanged, the run indexes it, prints
// no line about a write, and leaves no temporary.
func TestSETUP1_M1b_ARacingWriterIsReadNotOverwritten(t *testing.T) {
	dir := setupTree(t, setupApp)
	var fi os.FileInfo
	was := beforeSetupWrite
	beforeSetupWrite = func(root string) {
		p := setupPath(root)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(setupConfig), 0o644); err != nil {
			t.Fatal(err)
		}
		var err error
		if fi, err = os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { beforeSetupWrite = was })
	out, err := setupRun(dir)
	if err != nil || fi == nil || strings.Contains(out, "wrote ") || !strings.Contains(out, "indexed 2 file(s)") {
		t.Fatalf("graph index = %v (racer ran: %v), printed:\n%s\nwant the racer's file indexed and no write", err, fi != nil, out)
	}
	setupSame(t, setupPath(dir), []byte(setupConfig), fi)
	for _, n := range setupList(t, filepath.Join(dir, ".rashomon")) {
		if strings.Contains(n, ".tmp-") {
			t.Errorf(".rashomon holds the temporary %s; want it removed", n)
		}
	}
}

// TestSETUP1_A5_NoHookPathReachesCreateFile (pb38 a5): rootio.CreateFile has
// one caller outside rootio, graphConfig in graph_setup.go; graphConfig is
// called from graph.go alone (graphIndex), graphIndex from graph.go alone
// (cmdGraph and graphKnowledge), cmdGraph from graph.go alone
// (cmdBriefOrGraph), and that from main.go's verb switch alone: the
// person-typed `rashomon graph`, which no hook entry calls. Comment lines
// are not calls.
func TestSETUP1_A5_NoHookPathReachesCreateFile(t *testing.T) {
	repo := filepath.Join("..", "..")
	callers := map[string][]string{}
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(repo, top), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			rel, _ := filepath.Rel(repo, p)
			rel = filepath.ToSlash(rel)
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				for _, name := range []string{"CreateFile(", "graphConfig(", "graphIndex(", "cmdGraph(", "cmdBriefOrGraph("} {
					if strings.Contains(line, name) && !strings.HasPrefix(rel, "internal/rootio/") && !slices.Contains(callers[name], rel) {
						callers[name] = append(callers[name], rel)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	want := map[string][]string{
		"CreateFile(":      {"cmd/rashomon/graph_setup.go"},
		"graphConfig(":     {"cmd/rashomon/graph.go", "cmd/rashomon/graph_setup.go"},
		"graphIndex(":      {"cmd/rashomon/graph.go"},
		"cmdGraph(":        {"cmd/rashomon/graph.go"},
		"cmdBriefOrGraph(": {"cmd/rashomon/graph.go", "cmd/rashomon/main.go"},
	}
	for name, files := range want {
		got := callers[name]
		slices.Sort(got)
		if !slices.Equal(got, files) {
			t.Errorf("%s is in %v; want %v", name, got, files)
		}
	}
}
