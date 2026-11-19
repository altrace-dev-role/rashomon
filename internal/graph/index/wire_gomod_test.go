package index

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
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
