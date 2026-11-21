package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// envLine is the warning graph index prints for an environment at path.
func envLine(path string) string {
	return "warning: " + path + "/ not indexed: " + lang.LeftEnvironment + " (holds pyvenv.cfg)\n"
}

// putFiles writes under dir each slash path in files with its text.
func putFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
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

// TestGraphIndex_EnvironmentLinesStopAtMaxRefusedNamed (L2): `graph index`
// over a tree with n environments under its Python unit prints a warning
// line for each of the first graph.MaxRefusedNamed by path, the
// directories the header names, and for the rest one line that counts
// them, then the summary, whose file count holds no file under one; the
// header counts all n. Break: walk an environment; print a line for every
// one; write the count line at exactly MaxRefusedNamed.
func TestGraphIndex_EnvironmentLinesStopAtMaxRefusedNamed(t *testing.T) {
	for _, n := range []int{1, graph.MaxRefusedNamed, graph.MaxRefusedNamed + 1, graph.MaxRefusedNamed + 2} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			dir := t.TempDir()
			refusedPrintTree(t, dir)
			files := map[string]string{}
			var want strings.Builder
			for i := range n {
				env := fmt.Sprintf("host/e%02d", i)
				files[env+"/pyvenv.cfg"] = "home = /usr/bin\n"
				files[env+"/lib/m.py"] = "x = 1\n"
				if i < graph.MaxRefusedNamed {
					want.WriteString(envLine(env))
				}
			}
			putFiles(t, dir, files)
			if more := n - graph.MaxRefusedNamed; more > 0 {
				fmt.Fprintf(&want, "warning: %d more dir(s) not indexed\n", more)
			}
			want.WriteString("indexed 1 file(s): ")
			var out strings.Builder
			if err := graphIndex([]string{dir}, &out); err != nil {
				t.Fatalf("graph index = %v\n%s", err, out.String())
			}
			if got := out.String(); !strings.HasPrefix(got, want.String()) || strings.Count(got, "\n") != strings.Count(want.String(), "\n")+1 {
				t.Errorf("graph index printed:\n%s\nwant:\n%s...", got, want.String())
			}
			_, h, err := graph.LoadFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			if h.LeftOut != n || len(h.LeftOutDirs) != min(n, graph.MaxRefusedNamed) {
				t.Errorf("header left out %d, named %d; want %d, %d", h.LeftOut, len(h.LeftOutDirs), n, min(n, graph.MaxRefusedNamed))
			}
		})
	}
}

// TestGraphIndex_SkippedDirectoriesPrintOneCountLinePerKind (L2):
// `graph index` prints one count line for each kind of directory its walk
// skipped by name, dot directories (not the root's .rashomon), node_modules
// and __pycache__, in that order, after the environment lines and before
// the summary, and none for a kind it skipped none of; an environment under
// a dot name (.venv) is in the dot count, with no line of its own. Break:
// print a line at a count of 0; count .rashomon; name .venv.
func TestGraphIndex_SkippedDirectoriesPrintOneCountLinePerKind(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"every kind", map[string]string{
			".github/w.yml":                      "on: push\n",
			"host/.cache/c.py":                   "c = 1\n",
			".venv/pyvenv.cfg":                   "home = /usr/bin\n",
			".venv/lib/x.py":                     "x = 1\n",
			"node_modules/m/i.js":                "",
			"host/__pycache__/h.cpython-312.pyc": "",
			"host/venv/pyvenv.cfg":               "home = /usr/bin\n",
			"host/venv/m.py":                     "y = 1\n",
		}, envLine("host/venv") +
			"skipped 3 dir(s) whose name starts with a dot\n" +
			"skipped 1 node_modules dir(s)\n" +
			"skipped 1 __pycache__ dir(s)\n"},
		{"one kind", map[string]string{
			"host/__pycache__/h.cpython-312.pyc":     "",
			"host/sub/__pycache__/s.cpython-312.pyc": "",
		}, "skipped 2 __pycache__ dir(s)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			refusedPrintTree(t, dir)
			putFiles(t, dir, c.files)
			want := c.want + "indexed 1 file(s): "
			var out strings.Builder
			if err := graphIndex([]string{dir}, &out); err != nil {
				t.Fatalf("graph index = %v\n%s", err, out.String())
			}
			if got := out.String(); !strings.HasPrefix(got, want) || strings.Count(got, "\n") != strings.Count(want, "\n")+1 {
				t.Errorf("graph index printed:\n%s\nwant:\n%s...", got, want)
			}
		})
	}
}
