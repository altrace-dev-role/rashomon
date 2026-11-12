package gopg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGopg_BuiltinCallIsTheUniverse: builtinCall holds exactly the
// predeclared functions of the toolchain that builds this test, so a new
// builtin (as clear, min and max were in go1.21) fails here rather than
// landing in the calls attr as a name no declaration answers.
func TestGopg_BuiltinCallIsTheUniverse(t *testing.T) {
	n := 0
	for _, name := range types.Universe.Names() {
		_, isBuiltin := types.Universe.Lookup(name).(*types.Builtin)
		if isBuiltin {
			n++
		}
		if got := builtinCall(name); got != isBuiltin {
			t.Errorf("builtinCall(%q) = %v, want %v (go/types Universe)", name, got, isBuiltin)
		}
	}
	if n < 18 {
		t.Fatalf("go/types Universe holds %d builtin funcs, want at least 18: the comparison above is vacuous", n)
	}
	for _, name := range []string{"", "Append", "unsafe", "Sizeof", "foo"} {
		if builtinCall(name) {
			t.Errorf("builtinCall(%q) = true, want false", name)
		}
	}
}

// TestGopg_NoPackageLevelMap: gopg is on the hook path (the overlay), and
// a package-level map literal or make call is init code every hook process
// runs before main (TestRES2_InitBudget). Look-up tables here are switches.
func TestGopg_NoPackageLevelMap(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v, %d files", err, len(files))
	}
	fset, checked := token.NewFileSet(), 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				for _, v := range spec.(*ast.ValueSpec).Values {
					ast.Inspect(v, func(n ast.Node) bool {
						switch e := n.(type) {
						case *ast.CompositeLit:
							if _, isMap := e.Type.(*ast.MapType); isMap {
								t.Errorf("%s: package-level map literal", fset.Position(e.Pos()))
							}
						case *ast.CallExpr:
							if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "make" {
								t.Errorf("%s: package-level make", fset.Position(e.Pos()))
							}
						}
						return true
					})
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test file checked")
	}
}
