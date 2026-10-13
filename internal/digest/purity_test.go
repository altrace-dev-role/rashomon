package digest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// noFileAccess are the imports that let a package find or open a file on its
// own, rather than through the store it is handed.
var noFileAccess = map[string]bool{"os": true, "io/fs": true, "path/filepath": true, "bufio": true}

// sourceFiles parses one package directory's non-test files.
func sourceFiles(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = f
	}
	if len(out) == 0 {
		t.Fatalf("no source files in %s", dir)
	}
	return out
}

// readStepImports are the packages that open what a report reads beside the
// store: report's read step, and the project baseline it writes.
var readStepImports = map[string]bool{
	"github.com/altrace-dev-role/rashomon/internal/report/gather": true,
	"github.com/altrace-dev-role/rashomon/internal/baseline":      true,
}

// TestRubricHLD4_TheDigestOpensNoFileOfItsOwn is the direct-import half of
// the turn digest's promise that it never opens the session's transcript
// (package doc): digest, and the pure packages beside it -- shape and host --
// import nothing that opens a file, and digest imports neither report's read
// step nor the baseline. The transitive half, that nothing digest links can
// reach them either, is TestRubricHLD4_TheVerdictPackagesLinkNoReadStep.
func TestRubricHLD4_TheDigestOpensNoFileOfItsOwn(t *testing.T) {
	for _, dir := range []string{".", filepath.Join("..", "shape"), filepath.Join("..", "host")} {
		for name, f := range sourceFiles(t, dir) {
			for _, imp := range f.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if noFileAccess[path] || readStepImports[path] {
					t.Errorf("%s imports %s", name, path)
				}
			}
		}
	}
}
