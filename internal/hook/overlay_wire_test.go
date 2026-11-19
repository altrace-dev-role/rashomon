package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// wireImports is the import map index.Wire's Go resolver carries under
// root, or the refusal its Claim of a .go path there gives.
func wireImports(t *testing.T, root string) (*gopg.ImportMap, error) {
	t.Helper()
	r, ok, err := index.Wire().Claim(root, "a/a.go")
	if !ok {
		t.Fatalf("Wire claims no .go path under %s", root)
	}
	if err != nil {
		return nil, err
	}
	g, isGo := r.(gopg.Resolver)
	if !isGo {
		t.Fatalf("Wire's resolver for a .go path is %T, want gopg.Resolver", r)
	}
	return g.Imports, nil
}

// TestHook_WireGoMapsImportsAsTheOverlayDoes: under one root, the import
// map index.Wire's Go resolver carries and the one the hook builds an
// overlay with (overlayImports) are the same: the module path, and the
// answer for each directory asked, there or not; no map with no go.mod or
// no module line; and, where go.mod is there and refused, no map and one
// refusal, word for word. The hook does not read through Wire yet; this is
// what that move is checked against. Break: in Wire's Go step, wrap the
// refusal, or check directories under a root other than the one asked.
func TestHook_WireGoMapsImportsAsTheOverlayDoes(t *testing.T) {
	asked := []string{".", "a", "a/b", "zz", "go.mod"}
	for name, c := range map[string]struct {
		gomod   string // go.mod's text; "" with dir false: no go.mod
		dir     bool   // go.mod is a directory
		module  string // the overlay's module path; "": no map
		refused bool
	}{
		"a module":         {gomod: "module example.com/m\n\ngo 1.25.0\n", module: "example.com/m"},
		"no go.mod":        {},
		"no module line":   {gomod: "go 1.25.0\n"},
		"a refused go.mod": {dir: true, refused: true},
	} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
			t.Fatal(err)
		}
		gomod := filepath.Join(root, "go.mod")
		switch {
		case c.dir:
			if err := os.Mkdir(gomod, 0o755); err != nil {
				t.Fatal(err)
			}
		case c.gomod != "":
			if err := os.WriteFile(gomod, []byte(c.gomod), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, werr := overlayImports(root)
		if (want == nil) != (c.module == "") || (want != nil && want.Module != c.module) || (werr != nil) != c.refused {
			t.Fatalf("%s: overlayImports = %+v, %v; not what the fixture names", name, want, werr)
		}
		got, gerr := wireImports(t, root)
		if (gerr == nil) != (werr == nil) || (gerr != nil && gerr.Error() != werr.Error()) {
			t.Errorf("%s: Wire's refusal is %v, the overlay's %v; want one refusal", name, gerr, werr)
		}
		if (got == nil) != (want == nil) {
			t.Errorf("%s: Wire's map is %+v, the overlay's %+v; want both or neither", name, got, want)
			continue
		}
		if got == nil {
			continue
		}
		if got.Module != want.Module {
			t.Errorf("%s: Wire's module is %q, the overlay's %q", name, got.Module, want.Module)
		}
		for _, d := range asked {
			if g, w := got.IsDir(d), want.IsDir(d); g != w {
				t.Errorf("%s: IsDir(%q) is %v under Wire, %v under the overlay", name, d, g, w)
			}
		}
	}
}
