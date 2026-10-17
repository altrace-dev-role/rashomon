package verify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contractPackages are the directories P-07 adds; none is linked into the
// binary yet, so the RES-2 gate over `go list -deps` cannot see them.
var contractPackages = []string{
	"../verify", "../steer", "../steer/pattern", "../steer/render", "../steer/render/slot",
	"../budget", "../fold", "../disclose", "../spec", "../store",
}

// TestRES2c_NoPackageScopeRegexpInTheContracts: no contract package compiles
// a regexp at package scope, in a var initialiser or an init function
// (RES-2); a regexp is compiled on first use or not at all.
func TestRES2c_NoPackageScopeRegexpInTheContracts(t *testing.T) {
	fset := token.NewFileSet()
	files := 0
	for _, dir := range contractPackages {
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			for path, f := range pkg.Files {
				files++
				for _, pos := range packageScopeRegexpCompiles(f) {
					t.Errorf("%s: regexp compiled at package scope at %s", filepath.Clean(path), fset.Position(pos))
				}
			}
		}
	}
	if files < 20 {
		t.Fatalf("parsed %d files; the contract packages hold more than that", files)
	}
}

// packageScopeRegexpCompiles finds regexp.Compile, MustCompile and their
// POSIX forms in package-level var initialisers and in init bodies.
func packageScopeRegexpCompiles(f *ast.File) []token.Pos {
	var found []token.Pos
	visit := func(n ast.Node) {
		ast.Inspect(n, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "regexp" && strings.Contains(sel.Sel.Name, "Compile") {
				found = append(found, call.Pos())
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.VAR {
				for _, spec := range d.Specs {
					for _, v := range spec.(*ast.ValueSpec).Values {
						visit(v)
					}
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == "init" && d.Body != nil {
				visit(d.Body)
			}
		}
	}
	return found
}
