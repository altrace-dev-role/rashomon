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

// reportAdapters are report's exported functions that reach the filesystem:
// every exported function transcript_files.go declares, and Build, which
// reads the store, the transcripts and the sandbox trail.
func reportAdapters(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{"Build": true}
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "report", "transcript_files.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.IsExported() {
			out[fn.Name.Name] = true
		}
	}
	if !out["TranscriptIDs"] {
		t.Fatalf("premise: transcript_files.go declares no TranscriptIDs; adapters found %v", out)
	}
	return out
}

// TestRubricHLD4_TheDigestOpensNoFileOfItsOwn is the H-17-style dependency
// check for the turn digest's promise that it never opens the session's
// transcript (package doc): digest, and the pure packages beside it -- shape
// and host -- import nothing that opens a file, and digest calls none of
// report's filesystem adapters. Before this the promise held by convention
// only: digest links report, and report opens transcripts.
func TestRubricHLD4_TheDigestOpensNoFileOfItsOwn(t *testing.T) {
	adapters := reportAdapters(t)
	for _, dir := range []string{".", filepath.Join("..", "shape"), filepath.Join("..", "host")} {
		for name, f := range sourceFiles(t, dir) {
			for _, imp := range f.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if noFileAccess[path] {
					t.Errorf("%s imports %s", name, path)
				}
			}
			if dir != "." {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "report" && adapters[sel.Sel.Name] {
					t.Errorf("%s calls report.%s, which reaches the filesystem", name, sel.Sel.Name)
				}
				return true
			})
		}
	}
}
