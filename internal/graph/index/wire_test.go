package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// wireConfig declares the root as Go and inv as a Python unit owning the
// top-level module inv, as test/fixtures/lang/py-unit does.
const wireConfig = `{"units":[{"path":".","lang":"go"},{"path":"inv","mod_root":"","modules":["inv"],"lang":"py"}]}`

// wireRoot is a directory holding files, each a slash path and its text.
func wireRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, text := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// wireRefused is a root whose configuration file is a directory: a path
// that is there and is not a regular file, refused as a named pipe is, on
// every platform (the pipe itself is in wire_fifo_test.go, unix only).
func wireRefused(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(ConfigFile)), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestWire_RegistersGoThenPython: the hook's registry reads go, then
// py, the order the disclosed line names them in.
func TestWire_RegistersGoThenPython(t *testing.T) {
	if got, want := Wire().Langs(), []string{gopg.Lang, pylite.Lang}; !slices.Equal(got, want) {
		t.Errorf("Wire().Langs() = %v, want %v", got, want)
	}
}

// TestWire_PythonReadsThePythonUnits: under a root whose
// configuration declares a Go unit at the root and a py unit inv, a .py
// file under inv is unit:py:inv, and one outside inv is under no unit: the
// Go unit is not a Python unit. Break: keep every language's units.
func TestWire_PythonReadsThePythonUnits(t *testing.T) {
	root := wireRoot(t, map[string]string{ConfigFile: wireConfig})
	r, ok, err := Wire().Claim(root, "inv/core/calc.py")
	if !ok || err != nil {
		t.Fatalf("Claim = %v, %v, %v; want the Python resolver", r, ok, err)
	}
	if u, ok := r.Unit(root, "inv/core/calc.py"); !ok || u != graph.UnitID(pylite.Lang, "inv") {
		t.Errorf("Unit(inv/core/calc.py) = %q, %v; want unit:py:inv", u, ok)
	}
	if u, ok := r.Unit(root, "tools/gen.py"); ok {
		t.Errorf("Unit(tools/gen.py) = %q: a path under the Go unit only was given a Python unit", u)
	}
}

// TestWire_NoConfigurationIsNoPythonUnit: a root with no
// configuration file claims .py paths with no error and no unit. Break:
// read a missing file as a refusal.
func TestWire_NoConfigurationIsNoPythonUnit(t *testing.T) {
	root := t.TempDir()
	r, ok, err := Wire().Claim(root, "inv/core/calc.py")
	if !ok || err != nil || r == nil {
		t.Fatalf("Claim = %v, %v, %v; want the Python resolver and no error", r, ok, err)
	}
	if u, ok := r.Unit(root, "inv/core/calc.py"); ok {
		t.Errorf("with no configuration Unit = %q, want none", u)
	}
}

// TestWire_ARefusedConfigurationIsTheError: a configuration
// that is not a regular file, over MaxConfigBytes or not JSON refuses the
// root: the Claim of a .py path is nil and an error naming the file, its
// cause kept. Break: swallow the error.
func TestWire_ARefusedConfigurationIsTheError(t *testing.T) {
	for name, c := range map[string]struct {
		root  string
		cause func(error) bool
	}{
		"a directory": {wireRefused(t), func(err error) bool { return errors.Is(err, regfile.ErrNotRegular) }},
		"over the cap": {wireRoot(t, map[string]string{ConfigFile: `{"units":[]}` + strings.Repeat(" ", MaxConfigBytes)}),
			func(err error) bool {
				return strings.Contains(err.Error(), fmt.Sprintf("over %d bytes", MaxConfigBytes))
			}},
		"not JSON": {wireRoot(t, map[string]string{ConfigFile: "units: [inv]\n"}), func(err error) bool {
			var se *json.SyntaxError
			return errors.As(err, &se)
		}},
	} {
		r, ok, err := Wire().Claim(c.root, "inv/core/calc.py")
		if !ok || r != nil || err == nil {
			t.Errorf("%s: Claim = %v, %v, %v; want nil and an error", name, r, ok, err)
			continue
		}
		if !strings.Contains(err.Error(), ConfigFile) || !c.cause(err) {
			t.Errorf("%s: the error %q does not name %s with its cause", name, err, ConfigFile)
		}
	}
}

// TestWire_TwoRootsOneReadOneRefused (once per entry and root, on real files): in
// one registry one root's configuration reads and the other's is refused;
// each root's Claim gives its own answer, in either order, asked twice.
// Break: keep one result per language whatever the root.
func TestWire_TwoRootsOneReadOneRefused(t *testing.T) {
	read, refused := wireRoot(t, map[string]string{ConfigFile: wireConfig}), wireRefused(t)
	for _, order := range [][]string{{read, refused}, {refused, read}} {
		reg := Wire()
		for range 2 {
			for _, root := range order {
				r, _, err := reg.Claim(root, "inv/core/calc.py")
				if root == refused && (r != nil || !errors.Is(err, regfile.ErrNotRegular)) {
					t.Errorf("the refused root's Claim = %v, %v; want its refusal", r, err)
				}
				if root == read {
					if r == nil || err != nil {
						t.Fatalf("the read root's Claim = %v, %v; want its resolver", r, err)
					}
					if u, ok := r.Unit(root, "inv/core/calc.py"); !ok || u != graph.UnitID(pylite.Lang, "inv") {
						t.Errorf("the read root's Unit = %q, %v; want unit:py:inv", u, ok)
					}
				}
			}
		}
	}
}

// TestWire_GoCarriesImportsWhereThereIsAModule: under a root whose
// go.mod names a module, the Go resolver's overlay carries Declares and
// Imports; under a root with no go.mod, or a go.mod with no module line,
// Declares alone, and no error: as the hook's overlay import map gives
// today. A refused configuration file refuses nothing for Go. Break: give
// the resolver no import map where there is a module.
func TestWire_GoCarriesImportsWhereThereIsAModule(t *testing.T) {
	for name, c := range map[string]struct {
		root string
		want graph.Carried
	}{
		"a module":                {wireRoot(t, map[string]string{"go.mod": "module example.com/m\n\ngo 1.25.0\n"}), graph.Carried{graph.Declares, graph.Imports}},
		"no go.mod":               {t.TempDir(), graph.Carried{graph.Declares}},
		"no module line":          {wireRoot(t, map[string]string{"go.mod": "go 1.25.0\n"}), graph.Carried{graph.Declares}},
		"a refused configuration": {wireRefused(t), graph.Carried{graph.Declares}},
	} {
		r, ok, err := Wire().Claim(c.root, "a/a.go")
		if !ok || err != nil {
			t.Fatalf("%s: Claim = %v, %v, %v; want the Go resolver", name, r, ok, err)
		}
		if got := r.OverlayCarried(); !slices.Equal(got, c.want) {
			t.Errorf("%s: OverlayCarried() = %v, want %v", name, got, c.want)
		}
	}
}

// TestWire_AnUnclaimedPathReadsNothing: a path no language claims is
// false with no error, even under a root whose configuration is refused.
func TestWire_AnUnclaimedPathReadsNothing(t *testing.T) {
	if r, ok, err := Wire().Claim(wireRefused(t), "notes.md"); r != nil || ok || err != nil {
		t.Errorf("Claim(notes.md) = %v, %v, %v; want nil, false, nil", r, ok, err)
	}
}
