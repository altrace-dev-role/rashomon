package report

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// filesystemImports are the packages that let code find, open or write a
// file on its own. io/fs is not among them: its sentinel errors are values,
// and naming fs.ErrNotExist opens nothing.
var filesystemImports = map[string]bool{
	"os":            true,
	"path/filepath": true,
	"github.com/altrace-dev-role/rashomon/internal/baseline": true,
}

// TestRubricHLD4_NoReportFileReachesTheFilesystem holds report to being the
// verdict library: no file of it imports a package that opens a file, and
// none calls nono.Read, the sandbox trail's reader. Opening the transcripts,
// the trail and the project baseline is the read step's (internal/report/
// gather), which hands what it read to Assemble. digest and spend link
// report for its readers and verdict helpers; a report file that opened
// something again would hand them a path to a file they promise not to read.
func TestRubricHLD4_NoReportFileReachesTheFilesystem(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var checked int
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if filesystemImports[path] {
				t.Errorf("%s imports %s; opening a file belongs to the read step, internal/report/gather", name, path)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "nono" && sel.Sel.Name == "Read" {
				t.Errorf("%s calls nono.Read; the sandbox trail is read by the read step", name)
			}
			return true
		})
	}
	if checked < 10 {
		t.Fatalf("premise: checked only %d source files", checked)
	}
}
