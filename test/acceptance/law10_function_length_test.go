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

// functionLimit and fileLimit are the engineering constitution's Law 10
// ceilings: a hundred lines of code in one function and five hundred in one
// file, blank lines and comment lines not counted.
const (
	functionLimit = 100
	fileLimit     = 500
)

// overLimit are the functions that were already past the limit when this test
// was written, each at the size it had then. It is a ratchet, not a pardon: an
// entry may shrink and must then be lowered, it may never grow, and one that
// comes back under the limit must be removed. A function not listed here that
// crosses the limit fails outright.
var overLimit = map[string]int{
	"internal/shape/tokenize.go:lex":                  213,
	"internal/spend/spend.go:Build":                   177,
	"internal/shape/masked.go:lineParser.parseSimple": 116,
	"internal/spend/join.go:Summary.Join":             111,
}

// filesOverLimit is overLimit's ratchet for whole files.
var filesOverLimit = map[string]int{
	"cmd/rashomon/main.go":     1233,
	"internal/shape/shape.go":  1018,
	"internal/shape/masked.go": 940,
	"internal/report/text.go":  544,
}

// codeLines counts the lines of a function body strictly between its braces
// that hold code: neither blank nor wholly a // comment.
func codeLines(src []string, lbrace, rbrace int) int {
	n := 0
	for _, l := range src[lbrace:max(lbrace, rbrace-1)] {
		s := strings.TrimSpace(l)
		if s != "" && !strings.HasPrefix(s, "//") {
			n++
		}
	}
	return n
}

// functionSizes measures every function with a body in the module's non-test
// Go files under cmd/ and internal/, keyed file:Recv.Name, and every such
// file, keyed by its path.
func functionSizes(t *testing.T) (funcs, files map[string]int) {
	t.Helper()
	out, files := map[string]int{}, map[string]int{}
	fset := token.NewFileSet()
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(moduleRoot, top), func(p string, d fs.DirEntry, err error) error {
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
			rel, err := filepath.Rel(moduleRoot, p)
			if err != nil {
				return err
			}
			src := strings.Split(string(body), "\n")
			files[filepath.ToSlash(rel)] = codeLines(src, 0, len(src)+1)
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				out[filepath.ToSlash(rel)+":"+funcName(fn)] = codeLines(src,
					fset.Position(fn.Body.Lbrace).Line, fset.Position(fn.Body.Rbrace).Line)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out, files
}

// funcName is Name for a function and Recv.Name for a method.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if idx, ok := typ.(*ast.IndexExpr); ok {
		typ = idx.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// TestRubricLAW10_NoFunctionGrowsPastAHundredCodeLines holds every function in
// cmd/ and internal/ to Law 10. It was written when spend's readFile, already
// past the limit, grew by one more line in a change that was otherwise right:
// nothing measured the function, so nothing said it had moved the wrong way.
func TestRubricLAW10_NoFunctionGrowsPastAHundredCodeLines(t *testing.T) {
	sizes, _ := functionSizes(t)
	if len(sizes) < 500 {
		t.Fatalf("premise: measured only %d functions; the walk did not reach the module's code", len(sizes))
	}
	ratchet(t, sizes, overLimit, functionLimit)
}

// ratchet holds each size in sizes to limit, or, for a key listed in ceilings,
// to the size it had when it was listed.
func ratchet(t *testing.T, sizes, ceilings map[string]int, limit int) {
	t.Helper()
	for key, n := range sizes {
		ceiling, listed := ceilings[key]
		switch {
		case !listed && n > limit:
			t.Errorf("%s is %d code lines; Law 10 allows %d", key, n, limit)
		case listed && n > ceiling:
			t.Errorf("%s grew to %d code lines; it was %d and may only shrink", key, n, ceiling)
		case listed && n < ceiling:
			t.Errorf("%s shrank to %d code lines; lower its entry from %d (remove it at %d or fewer)",
				key, n, ceiling, limit)
		}
	}
	for key := range ceilings {
		if _, ok := sizes[key]; !ok {
			t.Errorf("the ratchet names %s, which no longer exists; remove the entry", key)
		}
	}
}

// TestRubricLAW10_NoFileGrowsPastFiveHundredCodeLines is the same ratchet
// over whole files. spend's scan.go was past the limit before this branch and
// grew by four lines with readFile's split; moving the decoded line's shape
// into line.go brought it back under, and this keeps it there.
func TestRubricLAW10_NoFileGrowsPastFiveHundredCodeLines(t *testing.T) {
	_, files := functionSizes(t)
	if len(files) < 50 {
		t.Fatalf("premise: measured only %d files; the walk did not reach the module's code", len(files))
	}
	ratchet(t, files, filesOverLimit, fileLimit)
}
