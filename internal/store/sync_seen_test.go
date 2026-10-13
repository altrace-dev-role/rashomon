package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestREC34j_EveryFullSyncGoesThroughSyncFull: the store issues a full sync in
// one place, syncFull, which tells the fault build's recorder before it syncs,
// and the stand-in barrier off darwin, which the recorder logs as the barrier
// it replaces. REC-34's acceptance test judges durability by what the
// recorder saw: a full sync issued anywhere else (an f.Sync() beside an
// accepted darwin barrier, a syscall.Fsync in a rewrite) puts the device
// flush back inside a lock where no test can see it. Every non-test file of
// the package is read whatever its build tags, so darwin's file is judged on
// Linux too (P09-F4).
func TestREC34j_EveryFullSyncGoesThroughSyncFull(t *testing.T) {
	allowed := map[string]bool{"store.go:syncFull": true, "sync_other.go:barrierSync": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if isFullSync(n) {
					found = append(found, name+":"+fn.Name.Name)
				}
				return true
			})
		}
	}
	sort.Strings(found)
	for _, where := range found {
		if !allowed[where] {
			t.Errorf("%s syncs in full by itself; only syncFull may, so that the recorder sees every full sync", where)
		}
	}
	if !slices.Contains(found, "store.go:syncFull") {
		t.Errorf("syncFull no longer calls Sync (found %q): the parse is not seeing what it should", found)
	}
}

// isFullSync reports a call that is a full sync: a method Sync taking no
// argument, as os.File's is, or syscall.Fsync.
func isFullSync(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name == "Sync" && len(call.Args) == 0 {
		return true
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "syscall" && sel.Sel.Name == "Fsync"
}
