package pylite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

var _ lang.Leaver = Resolver{}

// TestLeftOut_APyvenvCfgDirectoryIsAnEnvironment: a directory holding a
// regular file named pyvenv.cfg, at any depth, is left out as an
// environment, its reason the language-free kind and then the name the
// rule checked; a directory without one, and one where pyvenv.cfg is a
// directory, are not. Break: name no directory; name one by its name
// (venv) and not by the file; take any entry named pyvenv.cfg.
func TestLeftOut_APyvenvCfgDirectoryIsAnEnvironment(t *testing.T) {
	root := t.TempDir()
	for rel, text := range map[string]string{
		"venv/pyvenv.cfg":         "home = /usr/bin\n",
		"a/b/tools/pyvenv.cfg":    "home = /usr/bin\n",
		"src/app/__init__.py":     "",
		"odd/pyvenv.cfg/keep.txt": "",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env := lang.LeftEnvironment + " (holds pyvenv.cfg)"
	for _, c := range []struct {
		rel, why string
	}{
		{"venv", env},
		{"a/b/tools", env},
		{"a/b", ""},
		{"src", ""},
		{"src/app", ""},
		{"odd", ""},
	} {
		why, ok := Resolver{}.LeftOut(root, c.rel)
		if why != c.why || ok != (c.why != "") {
			t.Errorf("LeftOut(%s) = %q, %v; want %q, %v", c.rel, why, ok, c.why, c.why != "")
		}
	}
}
