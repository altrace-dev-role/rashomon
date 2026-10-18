package safe

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// canary is a panic value that stands for what a panic usually carries
// here: a string the program was holding, such as a command line or a
// prompt, either of which can hold a credential.
const canary = "CANARY-7f3a ghp_notarealtoken"

// TestRubricDOCS5_GuardNamesThePanicTypeOnly: the error Guard returns for a
// recovered panic names the value's type and nothing else. guarded() in
// cmd/rashomon prints this error to stderr for every CLI command, which is
// output people paste into issues (CWE-209).
func TestRubricDOCS5_GuardNamesThePanicTypeOnly(t *testing.T) {
	err := Guard(func() error { panic(canary) })
	assertTypeOnly(t, err, "string")

	err = Guard(func() error { panic(errors.New(canary)) })
	assertTypeOnly(t, err, "*errors.errorString")
}

// TestRubricDOCS5_GoNamesThePanicTypeOnly holds Go's onPanic to the same
// rule: the error it hands over names the type only.
func TestRubricDOCS5_GoNamesThePanicTypeOnly(t *testing.T) {
	var got error
	<-Go(func() { panic(canary) }, func(err error) { got = err })
	assertTypeOnly(t, got, "string")
}

func assertTypeOnly(t *testing.T, err error, typeName string) {
	t.Helper()
	if err == nil {
		t.Fatal("the panic was not reported")
	}
	var pe *PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("error %T is not a *PanicError", err)
	}
	if want := "recovered panic of type " + typeName; err.Error() != want {
		t.Errorf("error = %q, want exactly %q", err.Error(), want)
	}
	if strings.Contains(err.Error(), "CANARY") {
		t.Errorf("the panic value reached the error text: %q", err.Error())
	}
}

// TestRubricCONC5_OnlySafeStartsGoroutines: no non-test source file outside
// internal/safe contains a go statement. A goroutine started any other way
// carries no recover of its own, and a panic in it exits 2 -- which blocks
// the tool call -- whatever Guard is on the starting stack. Test files are
// exempt: they run under go test, not under Claude Code.
func TestRubricCONC5_OnlySafeStartsGoroutines(t *testing.T) {
	root := moduleRoot(t)
	walked := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(filepath.Dir(rel)) == "internal/safe" {
			return nil
		}
		walked[filepath.ToSlash(rel)] = true
		return checkNoGoStmt(t, path, rel)
	})
	if err != nil {
		t.Fatal(err)
	}
	// A count floor only fails once enough files drop out of the walk to
	// cross it, so a filter that excluded one real package, or a module root
	// resolved one level too high or too low, could still read as healthy
	// beside everything else the walk kept finding. Naming specific files
	// fails the moment any one of them stops being walked, whatever else
	// changed around it.
	for _, want := range mustWalk {
		if !walked[want] {
			t.Fatalf("%s was not walked under %s; the walk is not finding the module", want, root)
		}
	}
}

// mustWalk names source files TestRubricCONC5_OnlySafeStartsGoroutines must
// visit: one from cmd, one from a leaf internal package, so a module root
// resolved wrong or a filter that stopped descending into either tree is
// caught here.
var mustWalk = []string{
	"cmd/rashomon/main.go",
	"internal/hook/handle.go",
}

// TestUnsafeGoroutineStarts_DetectsAfterFuncAndGoStatements: the detector
// behind TestRubricCONC5_OnlySafeStartsGoroutines must also catch
// time.AfterFunc and context.AfterFunc, not just a go statement. Both run
// their function in a new goroutine with no recover of its own, exactly
// the hazard a go statement is -- before, the detector looked for *ast.GoStmt
// alone, so either call escaped it with no error and no test failure, in
// this package or anywhere else a contributor reached for one instead of
// safe.Go.
func TestUnsafeGoroutineStarts_DetectsAfterFuncAndGoStatements(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"a go statement", "package x\n\nfunc f() { go func() {}() }\n", 1},
		{"time.AfterFunc", "package x\n\nimport \"time\"\n\nfunc f() { time.AfterFunc(0, func() {}) }\n", 1},
		{"context.AfterFunc", "package x\n\nimport \"context\"\n\nfunc f(ctx context.Context) { context.AfterFunc(ctx, func() {}) }\n", 1},
		{"an unrelated call also named AfterFunc", "package x\n\nfunc f() { x.AfterFunc() }\n", 0},
		{"no goroutine at all", "package x\n\nfunc f() {}\n", 0},
		{"time imported under an alias", "package x\n\nimport t \"time\"\n\nfunc f() { t.AfterFunc(0, func() {}) }\n", 1},
		{"context imported under an alias", "package x\n\nimport c \"context\"\n\nfunc f(ctx c.Context) { c.AfterFunc(ctx, func() {}) }\n", 1},
		{"an import named time that is not the time package", "package x\n\nimport time \"example.com/other\"\n\nfunc f() { time.AfterFunc() }\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "x.go", tc.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			if got := unsafeGoroutineStarts(fset, f, "x.go"); len(got) != tc.want {
				t.Errorf("unsafeGoroutineStarts = %v, want %d report(s)", got, tc.want)
			}
		})
	}
}

// unsafeGoroutineStarts reports every way f starts a goroutine with no
// recover of its own: a go statement, or a call to time.AfterFunc or
// context.AfterFunc, which both run their function argument in a new
// goroutine outside whatever Guard is on the caller's stack. Each report
// names rel and the line.
func unsafeGoroutineStarts(fset *token.FileSet, f *ast.File, rel string) []string {
	afterFuncImports := afterFuncCapableImports(f)
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			out = append(out, fmt.Sprintf("%s:%d starts a goroutine with a go statement; use safe.Go, the only sanctioned way",
				rel, fset.Position(x.Pos()).Line))
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "AfterFunc" {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			path, ok := afterFuncImports[id.Name]
			if !ok {
				return true
			}
			out = append(out, fmt.Sprintf(
				"%s:%d starts a goroutine with %s.AfterFunc (%s), which carries no recover of its own; use safe.Go",
				rel, fset.Position(x.Pos()).Line, id.Name, path))
		}
		return true
	})
	return out
}

// afterFuncCapableImports maps each local identifier this file's imports
// bind to the real import path behind it, for every import whose path is
// "time" or "context" -- the two standard library packages whose AfterFunc
// starts a goroutine with no recover of its own. Matching on the import
// path rather than the identifier text catches time imported under an
// alias (import t "time"), which a literal name comparison let slip past,
// and does not falsely flag an identifier that merely happens to be named
// "time" or "context" while importing something else entirely.
func afterFuncCapableImports(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || (path != "time" && path != "context") {
			continue
		}
		local := path
		if imp.Name != nil {
			local = imp.Name.Name
		}
		out[local] = path
	}
	return out
}

// checkNoGoStmt parses one file, ignoring build tags so that every
// platform's and every tag's files are read, and reports each unsafe
// goroutine start.
func checkNoGoStmt(t *testing.T, path, rel string) error {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	for _, msg := range unsafeGoroutineStarts(fset, f, rel) {
		t.Error(msg)
	}
	return nil
}

// moduleRoot walks up from this package's directory to the one holding
// go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's directory")
		}
		dir = parent
	}
}
