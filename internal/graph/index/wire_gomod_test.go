package index

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// wireGo is the Go resolver Wire configures under root for a .go path.
func wireGo(t *testing.T, root string) gopg.Resolver {
	t.Helper()
	r, ok, err := Wire().Claim(root, "a/a.go")
	if !ok || err != nil {
		t.Fatalf("Claim(a/a.go) = %v, %v, %v; want the Go resolver", r, ok, err)
	}
	g, isGo := r.(gopg.Resolver)
	if !isGo {
		t.Fatalf("Claim(a/a.go) = %T, want gopg.Resolver", r)
	}
	return g
}

// TestWire_GoMapsUnderItsOwnModuleAndRoot: the Go resolver's import map
// carries the module path its root's go.mod names, and its directory check
// answers for directories under that root and no other: b is a directory
// under the other root only. Break: map under a module path not read from
// the root's go.mod, or check directories under another root.
func TestWire_GoMapsUnderItsOwnModuleAndRoot(t *testing.T) {
	root := wireRoot(t, map[string]string{"go.mod": "module example.com/m\n\ngo 1.25.0\n", "a/a.go": "package a\n"})
	other := wireRoot(t, map[string]string{"go.mod": "module example.com/o\n", "b/b.go": "package b\n"})
	for _, c := range []struct {
		root, module string
		dirs         map[string]bool
	}{
		{root, "example.com/m", map[string]bool{"a": true, "b": false, "zz": false}},
		{other, "example.com/o", map[string]bool{"a": false, "b": true, "zz": false}},
	} {
		g := wireGo(t, c.root)
		if g.Imports == nil || g.Imports.IsDir == nil {
			t.Fatalf("Imports = %+v under %s; want a map with a directory check", g.Imports, c.root)
		}
		if g.Imports.Module != c.module {
			t.Errorf("Imports.Module = %q, want %q", g.Imports.Module, c.module)
		}
		for dir, want := range c.dirs {
			if got := g.Imports.IsDir(dir); got != want {
				t.Errorf("under the root of %s, IsDir(%q) = %v, want %v", c.module, dir, got, want)
			}
		}
	}
}

// TestWire_AGoModOverTheBoundReadsItsCompleteLines: a go.mod longer than
// gopg.MaxGoModBytes is not refused; its complete lines under the bound are
// read, so a module line at the top still gives the map, with no error.
// Break: refuse a go.mod over the bound.
func TestWire_AGoModOverTheBoundReadsItsCompleteLines(t *testing.T) {
	gomod := "module example.com/m\n\n" + strings.Repeat("// a line that pads go.mod past the bound\n", gopg.MaxGoModBytes/40+1)
	if len(gomod) <= gopg.MaxGoModBytes {
		t.Fatalf("the fixture is %d bytes, not over the bound %d", len(gomod), gopg.MaxGoModBytes)
	}
	g := wireGo(t, wireRoot(t, map[string]string{"go.mod": gomod}))
	if g.Imports == nil || g.Imports.Module != "example.com/m" {
		t.Errorf("Imports = %+v; want module example.com/m", g.Imports)
	}
}

// TestWire_AGoModDirectoryRefusesGo: a go.mod that is a directory is there
// and is not a regular file, refused as a named pipe is, on every platform:
// Go under that root is refused with an error naming go.mod, with
// regfile.ErrNotRegular kept. Break: let the Go step drop the error.
func TestWire_AGoModDirectoryRefusesGo(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, ok, err := Wire().Claim(root, "a/a.go")
	if !ok || r != nil || !errors.Is(err, regfile.ErrNotRegular) || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("Claim(a/a.go) = %v, %v, %v; want nil and an error naming go.mod, ErrNotRegular kept", r, ok, err)
	}
}
