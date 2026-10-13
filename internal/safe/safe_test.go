package safe

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
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
	files := 0
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
		files++
		return checkNoGoStmt(t, path, rel)
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 50 {
		t.Fatalf("walked %d source files under %s; the walk is not finding the module", files, root)
	}
}

// checkNoGoStmt parses one file, ignoring build tags so that every
// platform's and every tag's files are read, and reports each go statement.
func checkNoGoStmt(t *testing.T, path, rel string) error {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if g, ok := n.(*ast.GoStmt); ok {
			t.Errorf("%s:%d starts a goroutine with a go statement; use safe.Go, the only sanctioned way",
				rel, fset.Position(g.Pos()).Line)
		}
		return true
	})
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
