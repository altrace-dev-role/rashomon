package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestREC34h_EveryStoreLockAndSyncIsRecorded: the sync recorder REC-34 reads
// is only as good as what it is shown. Every lock the store takes goes
// through lockCounted, which the fault build counts, so a sync inside any of
// them -- forget's, eviction's, a reader's -- is logged as inside a lock; and
// every sync the store issues goes through syncFull or barrierSync, which log
// it, so none is invisible. Read from the source of every platform's files,
// whatever this build's tags, because a lock_windows.go call site is as much
// a gap as a lock_unix.go one.
func TestREC34h_EveryStoreLockAndSyncIsRecorded(t *testing.T) {
	allowed := map[string]map[string]bool{
		"LockFile": {"lockCounted": true},
		"Sync":     {"syncFull": true, "barrierSync": true},
	}
	var bad []string
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					if fun.Name == "LockFile" {
						name = fun.Name
					}
				case *ast.SelectorExpr:
					if fun.Sel.Name == "Sync" && len(call.Args) == 0 {
						name = fun.Sel.Name
					}
				}
				if name != "" && !allowed[name][fn.Name.Name] {
					bad = append(bad, fset.Position(call.Pos()).String()+": "+name+" in "+fn.Name.Name)
				}
				return true
			})
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("%s: the sync recorder cannot see it; take the lock through lockCounted and sync through syncFull", b)
	}
}
