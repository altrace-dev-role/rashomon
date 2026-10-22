package acceptance

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

// TestRubricCONC7_ReadRecapPayloadHasExactlyOneCaller holds readRecapPayload's
// doc comment to something more than words: "call it only from such a
// process, once" names a constraint nothing in the type system enforces, since
// the goroutine it starts can outlive any caller that is not cmdRecap's
// short-lived process (TestRubricCONC7_AbandonedStdinReadEndsWhenTheReadReturns).
// A second caller would mean a second place that promise has to be checked by
// a person instead. This makes it structural: the test fails the day a second
// non-test caller appears, rather than staying silent until someone notices.
func TestRubricCONC7_ReadRecapPayloadHasExactlyOneCaller(t *testing.T) {
	const callee = "readRecapPayload"
	fset := token.NewFileSet()
	calls := 0
	err := filepath.WalkDir(filepath.Join(moduleRoot, "cmd"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, body, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// readRecapPayload is unexported, so every real call site names
			// it as a bare identifier; a selector would be a different
			// package's function of the same name, not this one.
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == callee {
				calls++
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The one call this counts is readRecapPayload's own declaration's
	// signature appearing nowhere as a CallExpr, and cmdRecap's single call
	// at the time this test was written.
	if calls != 1 {
		t.Fatalf("%s has %d callers in cmd/, want exactly 1 -- its doc comment's bound on the goroutine it starts assumes a single short-lived caller", callee, calls)
	}
}
