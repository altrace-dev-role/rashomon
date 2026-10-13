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

// watchedCalls is the one allowlist of the store's source check: for each
// kind of call the sync recorder depends on, the file:function sites that may
// make it, every one of which must exist. A lock is taken only through
// lockCounted, which the fault build counts. A full sync -- a Sync() method
// taking no argument, as os.File's is, or syscall.Fsync -- is issued only by
// syncFull, which tells the recorder first, and by the stand-in barrier off
// darwin, which the recorder logs as the barrier it replaces. The darwin
// barrier's fcntl is issued only by fcntlBarrier.
var watchedCalls = map[string][]string{
	"lockFile":  {"store.go:lockCounted"},
	"full sync": {"store.go:syncFull", "sync_other.go:barrierSync"},
	"fcntl":     {"sync_darwin.go:fcntlBarrier"},
}

// TestREC34h_EveryStoreLockAndSyncIsRecorded: the sync recorder REC-34 reads
// is only as good as what it is shown. Every lock the store takes goes
// through lockCounted, so a sync inside any of them -- forget's, eviction's,
// a reader's -- is logged as inside a lock; every full sync goes through
// syncFull, so none is invisible (an f.Sync() beside an accepted darwin
// barrier would put the device flush back inside the lock while the recorder
// saw only the barrier); and darwin's barrier is fcntl's F_BARRIERFSYNC,
// named fBarrierFsync and 85, with no F_FULLFSYNC anywhere in the store's
// code, since a barrier the kernel accepts as a full sync is logged as a
// barrier too (P09-F4, P09-F8). Every non-test file is read whatever this
// build's tags, so darwin's file is judged on Linux too, and a
// lock_windows.go call site is as much a gap as a lock_unix.go one.
func TestREC34h_EveryStoreLockAndSyncIsRecorded(t *testing.T) {
	sites, src := storeCallSites(t)
	for kind, allowed := range watchedCalls {
		for _, site := range sites[kind] {
			if !slices.Contains(allowed, site.where) {
				t.Errorf("%s: %s in %s, where the sync recorder cannot see it; only %v may", site.pos, kind, site.where, allowed)
			}
		}
		for _, want := range allowed {
			if !slices.ContainsFunc(sites[kind], func(s callSite) bool { return s.where == want }) {
				t.Errorf("no %s in %s (found %v): the parse is not seeing what it should", kind, want, sites[kind])
			}
		}
	}
	for _, site := range sites["fcntl"] {
		if site.arg != "fBarrierFsync" {
			t.Errorf("%s: fcntl's command is %s, want fBarrierFsync: anything else is not the barrier the recorder logs", site.pos, site.arg)
		}
	}
	for _, pos := range src.fullFsync {
		t.Errorf("%s: F_FULLFSYNC in the store's code; a full sync is syncFull's, through os.File.Sync", pos)
	}
	if src.barrierValue != "85" {
		t.Errorf("fBarrierFsync is %q, want 85, F_BARRIERFSYNC in <sys/fcntl.h>", src.barrierValue)
	}
}

// callSite is one watched call: where it is, as file:function, its position,
// and for fcntl the command it passes.
type callSite struct {
	where, pos, arg string
}

// sourceFacts are what the check reads outside the call sites: every use of
// the identifier F_FULLFSYNC, and the value fBarrierFsync is declared with.
type sourceFacts struct {
	fullFsync    []string
	barrierValue string
}

// storeCallSites parses every non-test file of the package and returns the
// watched calls by kind, and the source facts.
func storeCallSites(t *testing.T) (map[string][]callSite, sourceFacts) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sites := map[string][]callSite{}
	var facts sourceFacts
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
		ast.Inspect(file, func(n ast.Node) bool {
			facts.note(fset, n)
			return true
		})
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if kind, arg := watchedKind(call); kind != "" {
					sites[kind] = append(sites[kind], callSite{where: name + ":" + fn.Name.Name, pos: fset.Position(call.Pos()).String(), arg: arg})
				}
				return true
			})
		}
	}
	for _, s := range sites {
		sort.Slice(s, func(i, j int) bool { return s[i].pos < s[j].pos })
	}
	return sites, facts
}

// note records an F_FULLFSYNC identifier, bare or qualified, and the value
// of the constant fBarrierFsync.
func (f *sourceFacts) note(fset *token.FileSet, n ast.Node) {
	switch n := n.(type) {
	case *ast.Ident:
		if n.Name == "F_FULLFSYNC" {
			f.fullFsync = append(f.fullFsync, fset.Position(n.Pos()).String())
		}
	case *ast.ValueSpec:
		for i, id := range n.Names {
			if id.Name == "fBarrierFsync" && i < len(n.Values) {
				if lit, ok := n.Values[i].(*ast.BasicLit); ok {
					f.barrierValue = lit.Value
				}
			}
		}
	}
}

// watchedKind classifies a call: "lockFile"; "full sync", a Sync method
// taking no argument or syscall.Fsync; "fcntl", a raw syscall whose number is
// syscall.SYS_FCNTL, with the command it passes; or "" for anything else.
func watchedKind(call *ast.CallExpr) (kind, arg string) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if fun.Name == "lockFile" {
			return "lockFile", ""
		}
	case *ast.SelectorExpr:
		pkg, _ := fun.X.(*ast.Ident)
		switch {
		case fun.Sel.Name == "Sync" && len(call.Args) == 0:
			return "full sync", ""
		case pkg != nil && pkg.Name == "syscall" && fun.Sel.Name == "Fsync":
			return "full sync", ""
		case pkg != nil && pkg.Name == "syscall" && strings.HasPrefix(fun.Sel.Name, "Syscall") && len(call.Args) >= 3 && isSysFcntl(call.Args[0]):
			return "fcntl", exprString(call.Args[2])
		}
	}
	return "", ""
}

// isSysFcntl reports syscall.SYS_FCNTL.
func isSysFcntl(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "syscall" && sel.Sel.Name == "SYS_FCNTL"
}

// exprString renders an fcntl command: an identifier, a qualified one, or a
// literal; anything else reads as what it is, an expression.
func exprString(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			return pkg.Name + "." + e.Sel.Name
		}
	case *ast.BasicLit:
		return e.Value
	}
	return "an expression"
}
