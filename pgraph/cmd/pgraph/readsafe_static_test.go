package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// modeByPath names the functions that change a file's mode, owner or times
// by its path, as os, syscall and golang.org/x/sys/unix spell them.
var modeByPath = map[string]bool{"Chmod": true, "Lchmod": true, "Chown": true, "Lchown": true, "Chtimes": true}

// modeByPathCalls lists, as file:line:col, every call os.X, syscall.X or
// unix.X with X in modeByPath in the module's .go files under dir: its
// _test.go files when tests is true, the others when false. It walks as the
// go command does, skipping testdata and directories that start with . or _.
func modeByPathCalls(t *testing.T, dir string, tests bool) []string {
	t.Helper()
	var out []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); p != dir && (n == "testdata" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") != tests {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			s, ok := c.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := s.X.(*ast.Ident); ok && (x.Name == "os" || x.Name == "syscall" || x.Name == "unix") && modeByPath[s.Sel.Name] {
				out = append(out, fset.Position(c.Pos()).String())
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestRS1a_R22_NoModeChangeByPath: the pgraph module's non-test files change
// no file's mode, owner or times by path; a sync sets its files' mode on the
// open handle, so the mode lands on the file the sync wrote and not on
// whatever the name points at by then. R20 reads the mode and cannot tell
// how it was set; this row pins that. The control: the same scan over the
// module's test files finds such a call, so the scan can see one. Break:
// put os.Chmod(tmp, 0o644) back in the sync, and this fails.
func TestRS1a_R22_NoModeChangeByPath(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the pgraph module root: %v", err)
	}
	if got := modeByPathCalls(t, root, true); len(got) == 0 {
		t.Fatal("control: the scan found no mode change by path in the module's test files, so it cannot see one")
	}
	if got := modeByPathCalls(t, root, false); len(got) != 0 {
		t.Fatalf("the pgraph module changes a mode by path at %s; set it on the handle", strings.Join(got, ", "))
	}
}
