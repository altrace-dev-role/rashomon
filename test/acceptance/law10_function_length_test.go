package acceptance

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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
//
// usage and defaultFiles are at their sizes on main when this ratchet landed
// there: both grew under it on main (new commands, and the store files the
// stack beneath it added to the inventory) while it waited on its branch.
var overLimit = map[string]int{
	"internal/spend/spend.go:Build":              177,
	"internal/spend/join.go:Summary.Join":        111,
	"cmd/rashomon/main.go:usage":                 106,
	"internal/disclose/disclose.go:defaultFiles": 263,
}

// filesOverLimit is overLimit's ratchet for whole files, at their sizes on
// main when this ratchet landed there: main.go, store.go and watch.go grew on
// main while it waited on its branch.
var filesOverLimit = map[string]int{
	"cmd/rashomon/main.go":    1299,
	"internal/store/store.go": 563,
	"cmd/rashomon/watch.go":   545,
}

// codeLines counts the lines strictly between lbrace and rbrace (1-indexed,
// as go/token reports them) that hold at least one token that is not a
// comment: a line wholly inside a /* */ block comment is not code, however
// many lines that block spans, the same as a line that is wholly a //
// comment; a line that holds both code and a trailing comment still counts,
// on its code token alone.
//
// Tokenised rather than matched against "//": a scanner run with comments
// off (the default scanner.Mode) never emits a token for one, block or
// line, so a line with no token at all is a line with no code on it,
// whatever a textual prefix check would have had to special-case.
func codeLines(src []string, lbrace, rbrace int) int {
	seg := src[lbrace:max(lbrace, rbrace-1)]
	body := []byte(strings.Join(seg, "\n"))
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(body))
	var sc scanner.Scanner
	sc.Init(file, body, nil, 0)
	lines := map[int]bool{}
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		start := fset.Position(pos).Line
		end := start
		if tok == token.STRING {
			// The one multi-line token left once comments are skipped: a
			// backtick string spanning several lines is code on every one
			// of them, counted the same way the line-prefix check it
			// replaced counted them -- except a BLANK line inside one,
			// which that check never counted either (it tested the line's
			// text, not what token holds it), and the usage string's own
			// blank lines must not grow its function's count.
			// (token.SEMICOLON's own lit is "\n" for one go/scanner
			// auto-inserts, which is not a second line's worth of code and
			// must not be read as one -- hence STRING alone, here.)
			end += strings.Count(lit, "\n")
		}
		for l := start; l <= end; l++ {
			if l-1 < len(seg) && strings.TrimSpace(seg[l-1]) == "" {
				continue
			}
			lines[l] = true
		}
	}
	return len(lines)
}

// measureFile parses one Go source file's bytes and returns the code-line
// count of every function it declares, and the file's own code-line count.
//
// A function's key is relPath:funcName the first time that name is seen in
// the file, and relPath:funcName#2, #3, ... for each one after: Go allows
// more than one init() per file, and the bare name alone -- every function
// shares it, since init has no receiver to tell two apart by -- silently
// dropped every one but the last seen.
func measureFile(fset *token.FileSet, relPath string, body []byte) (funcs map[string]int, fileLines int, err error) {
	f, err := parser.ParseFile(fset, relPath, body, parser.SkipObjectResolution)
	if err != nil {
		return nil, 0, err
	}
	src := strings.Split(string(body), "\n")
	funcs = map[string]int{}
	seen := map[string]int{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := funcName(fn)
		seen[name]++
		key := relPath + ":" + name
		if n := seen[name]; n > 1 {
			key += "#" + strconv.Itoa(n)
		}
		funcs[key] = codeLines(src, fset.Position(fn.Body.Lbrace).Line, fset.Position(fn.Body.Rbrace).Line)
	}
	return funcs, codeLines(src, 0, len(src)+1), nil
}

// functionSizes measures every function with a body in the module's non-test
// Go files under cmd/ and internal/, keyed file:Recv.Name (measureFile), and
// every such file, keyed by its path.
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
			rel, err := filepath.Rel(moduleRoot, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			fileFuncs, fileLines, err := measureFile(fset, rel, body)
			if err != nil {
				return err
			}
			files[rel] = fileLines
			for key, n := range fileFuncs {
				out[key] = n
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out, files
}

// funcName is Name for a function and Recv.Name for a method. A receiver
// with one type parameter parses as *ast.IndexExpr; two or more parse as
// *ast.IndexListExpr, a distinct node type unwrapped the same way, so a
// method on a two-type-parameter receiver keys on its own type rather than
// colliding with a same-named method on an unrelated one-parameter type.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	switch idx := typ.(type) {
	case *ast.IndexExpr:
		typ = idx.X
	case *ast.IndexListExpr:
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
