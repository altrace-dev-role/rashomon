package nono

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestRubricCONC3_NoExportedFunctionOpensAFile holds this package's split
// between a parser and an opener: Observe takes an already-open reader, and
// the function that calls os.Open (read) is unexported, so a caller that
// links this package only for its types -- report does, for Observation;
// digest does not link it at all -- has no exported path from that import
// to a sandbox trail's bytes. Opening the trail is the read step's own
// (internal/report/gather), which is why read has no exported twin.
func TestRubricCONC3_NoExportedFunctionOpensAFile(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var checked int
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !fn.Name.IsExported() {
				continue
			}
			checked++
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" &&
					(sel.Sel.Name == "Open" || sel.Sel.Name == "ReadFile" || sel.Sel.Name == "OpenFile") {
					t.Errorf("%s: exported func %s calls os.%s; the file opener must stay unexported", name, fn.Name.Name, sel.Sel.Name)
				}
				return true
			})
		}
	}
	if checked == 0 {
		t.Fatal("premise: no exported function with a body was checked")
	}
}
