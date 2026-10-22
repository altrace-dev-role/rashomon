package verify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// contractPackages are the directories P-07 adds; none is linked into the
// binary yet, so the RES-2 gate over `go list -deps` cannot see them.
var contractPackages = []string{
	"../verify", "../steer", "../steer/pattern", "../steer/render", "../steer/render/slot",
	"../budget", "../fold", "../disclose", "../spec", "../store",
}

// TestRES2c_NoPackageScopeRegexpInTheContracts: no contract package compiles
// a regexp at package scope, in a var initialiser or an init function
// (RES-2); a regexp is compiled on first use or not at all. A dot import of
// "regexp" is refused outright: it puts Compile and MustCompile within
// reach of a bare identifier this check cannot tell apart from any other
// package-level call.
func TestRES2c_NoPackageScopeRegexpInTheContracts(t *testing.T) {
	fset := token.NewFileSet()
	files := 0
	for _, dir := range contractPackages {
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			var named []*ast.File
			for path, f := range pkg.Files {
				files++
				if dotImportsRegexp(f) {
					t.Errorf("%s: dot-imports \"regexp\"; name it so this check can follow its calls", filepath.Clean(path))
					continue
				}
				named = append(named, f)
			}
			for _, pos := range packageScopeRegexpCompiles(named) {
				t.Errorf("%s: regexp compiled at package scope", fset.Position(pos))
			}
		}
	}
	if files < 20 {
		t.Fatalf("parsed %d files; the contract packages hold more than that", files)
	}
}

// regexpLocalName is the identifier an import of "regexp" is reached
// through in f: "regexp" itself when the import has no alias, the alias
// when it has one, and "" when the file does not import it at all (or
// imports it only blank, which never calls anything).
func regexpLocalName(f *ast.File) string {
	for _, imp := range f.Imports {
		if path, err := strconv.Unquote(imp.Path.Value); err != nil || path != "regexp" {
			continue
		}
		switch {
		case imp.Name == nil:
			return "regexp"
		case imp.Name.Name == "_" || imp.Name.Name == ".":
			return ""
		default:
			return imp.Name.Name
		}
	}
	return ""
}

// dotImportsRegexp reports whether f imports "regexp" with ".".
func dotImportsRegexp(f *ast.File) bool {
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err == nil && path == "regexp" && imp.Name != nil && imp.Name.Name == "." {
			return true
		}
	}
	return false
}

// packageScopeRegexpCompiles finds calls through a file's own name for
// "regexp" (plain or aliased) to Compile, MustCompile and their POSIX
// forms, in package-level var initialisers and in init bodies across the
// files of one package. A var initialiser that calls a same-package
// function declared in any of those files is followed into that function's
// body, which is read through its own file's name for "regexp", so a
// compile hidden behind a helper in another file is not missed; the search
// does not revisit a function already on the call path, so a recursive
// helper cannot loop it.
func packageScopeRegexpCompiles(files []*ast.File) []token.Pos {
	type helper struct {
		decl       *ast.FuncDecl
		regexpName string
	}
	funcs := map[string]helper{}
	for _, f := range files {
		name := regexpLocalName(f)
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				funcs[fn.Name.Name] = helper{fn, name}
			}
		}
	}

	var found []token.Pos
	var visit func(n ast.Node, regexpName string, seen map[string]bool)
	visit = func(n ast.Node, regexpName string, seen map[string]bool) {
		ast.Inspect(n, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if id, ok := fun.X.(*ast.Ident); ok && regexpName != "" && id.Name == regexpName && strings.Contains(fun.Sel.Name, "Compile") {
					found = append(found, call.Pos())
				}
			case *ast.Ident:
				if callee, ok := funcs[fun.Name]; ok && callee.decl.Body != nil && !seen[fun.Name] {
					next := map[string]bool{fun.Name: true}
					for k := range seen {
						next[k] = true
					}
					visit(callee.decl.Body, callee.regexpName, next)
				}
			}
			return true
		})
	}
	for _, f := range files {
		name := regexpLocalName(f)
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					for _, spec := range d.Specs {
						for _, v := range spec.(*ast.ValueSpec).Values {
							visit(v, name, map[string]bool{})
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" && d.Body != nil {
					visit(d.Body, name, map[string]bool{})
				}
			}
		}
	}
	return found
}

// TestRES2e_HelperInAnotherFileIsFollowed: a var initialiser in one file of
// a package that calls a helper declared in another file is followed into
// that helper, and the helper's compile is read through its own file's name
// for "regexp", so a compile split across two files is caught; a file that
// calls a helper compiling nothing stays clean.
func TestRES2e_HelperInAnotherFileIsFollowed(t *testing.T) {
	parse := func(src string) *ast.File {
		f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	vars := parse("package p\n\nvar form = compileForm()\n\nvar plain = plainHelper()\n")
	helpers := parse("package p\n\nimport re \"regexp\"\n\nfunc compileForm() *re.Regexp { return re.MustCompile(`x`) }\n\nfunc plainHelper() int { return len(`x`) }\n")
	if got := packageScopeRegexpCompiles([]*ast.File{vars, helpers}); len(got) != 1 {
		t.Errorf("found %d package-scope compiles, want 1: the helper in the other file", len(got))
	}
	if got := packageScopeRegexpCompiles([]*ast.File{parse("package p\n\nvar plain = plainHelper()\n"), helpers}); len(got) != 0 {
		t.Errorf("found %d compiles behind a helper that compiles nothing, want 0", len(got))
	}
}
