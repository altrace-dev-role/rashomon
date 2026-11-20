package learn

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// learnPath is this package's import path, as other packages import it.
const learnPath = "github.com/altrace-dev-role/rashomon/internal/learn"

// TestRH7c_OnlyLearnImportAddsALesson is a screen for the report's words "no
// lesson miner exists", not a proof of them. It parses every non-test Go
// file under cmd and internal and fails on three shapes only:
//   - an assignment whose target is a ledger's lesson list (x.file.Lessons),
//     one row of it (x.file.Lessons[i]) or, in this package, the whole
//     lessons file (x.file), anywhere but (*Ledger).Import (an edit of a
//     field of one row, such as its tier, is not such a target);
//   - a selector named Import on anything but a package name, anywhere but
//     importLessons, the learn import verb (a package's own Import
//     function, such as knowledge's, is not counted);
//   - a reference to Candidate, the check a miner would run before it
//     proposed a lesson: learn.Candidate by its import name, or Candidate
//     in this package other than its declaration's name.
//
// What it cannot see: a dot-import of this package; a write through a
// pointer or an alias of the list; copy into the list; a miner under
// another name that reaches neither Import nor Candidate; Go source outside
// cmd and internal; a lessons.json written by path, or by hand, outside the
// ledger (the ledger reads such a file as unsigned and quarantined, and the
// report says the ledger is unchecked); reflection; and a package that
// shadows a package name with a variable.
func TestRH7c_OnlyLearnImportAddsALesson(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var files int
	var writes, imports, candidates []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			files++
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			names := importNames(t, f)
			inLearn := path.Dir(rel) == "internal/learn"
			for _, decl := range f.Decls {
				at, own := rel+" (package level)", (*ast.Ident)(nil)
				if fn, ok := decl.(*ast.FuncDecl); ok {
					at, own = rel+" "+fn.Name.Name, fn.Name
				}
				ast.Inspect(decl, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.AssignStmt:
						for _, lhs := range n.Lhs {
							if writesLessonList(lhs, inLearn) {
								writes = append(writes, at)
							}
						}
					case *ast.SelectorExpr:
						x, isIdent := n.X.(*ast.Ident)
						_, isPkg := names[identName(x)]
						switch {
						case n.Sel.Name == "Import" && !(isIdent && isPkg):
							imports = append(imports, at)
						case n.Sel.Name == "Candidate" && isIdent && names[x.Name] == learnPath:
							candidates = append(candidates, at)
						}
					case *ast.Ident:
						if inLearn && n.Name == "Candidate" && n != own {
							candidates = append(candidates, at)
						}
					}
					return true
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 100 {
		t.Fatalf("parsed %d files under cmd and internal; want the module's, over 100", files)
	}
	if want := []string{"internal/learn/ledger.go Import"}; !slices.Equal(writes, want) {
		t.Errorf("a ledger's lesson list is written at %q; want only %q", writes, want)
	}
	if want := []string{"cmd/rashomon/learn.go importLessons"}; !slices.Equal(imports, want) {
		t.Errorf("a method named Import is reached at %q; want only %q", imports, want)
	}
	if len(candidates) != 0 {
		t.Errorf("Candidate is called at %q; want no caller", candidates)
	}
}

// importNames maps each name a file's imports bind to the import's path. An
// unnamed import binds its path's last element, as this module's packages
// are named.
func importNames(t *testing.T, f *ast.File) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		out[name] = p
	}
	return out
}

// identName is x's name, or "" for no identifier.
func identName(x *ast.Ident) string {
	if x == nil {
		return ""
	}
	return x.Name
}

// writesLessonList reports an assignment target that is a ledger's lesson
// list (x.file.Lessons), one row of it (x.file.Lessons[i]), or, in this
// package, where the ledger's file field is, the whole lessons file (x.file).
func writesLessonList(e ast.Expr, inLearn bool) bool {
	if ix, ok := e.(*ast.IndexExpr); ok {
		e = ix.X
	}
	s, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if s.Sel.Name == "file" {
		return inLearn
	}
	inner, ok := s.X.(*ast.SelectorExpr)
	return ok && s.Sel.Name == "Lessons" && inner.Sel.Name == "file"
}
