package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestConflictKind_ClosedList pins the conflict kinds: ConflictKinds lists
// every ConflictKind constant once and nothing else, each spelled in lower
// case and underscores, arity first and then the five of the knowledge and
// session graphs.
func TestConflictKind_ClosedList(t *testing.T) {
	consts := conflictKindConsts(t)
	for i, k := range ConflictKinds {
		if _, declared := consts[k]; !declared {
			t.Errorf("%q listed but not declared", k)
		}
		if slices.Index(ConflictKinds, k) != i {
			t.Errorf("%q listed twice", k)
		}
		if k == "" || strings.TrimFunc(string(k), func(r rune) bool { return r >= 'a' && r <= 'z' || r == '_' }) != "" {
			t.Errorf("%q is not lower-case letters and underscores", k)
		}
	}
	for k, name := range consts {
		if !slices.Contains(ConflictKinds, k) {
			t.Errorf("%s = %q declared but not listed", name, k)
		}
	}
	want := []ConflictKind{ConflictArity, ConflictBoundaryCross, ConflictDecisionContradicted,
		ConflictSignatureMismatch, ConflictDuplicateAbstraction, ConflictProtectedPath}
	if !slices.Equal(ConflictKinds, want) {
		t.Errorf("ConflictKinds = %q, want %q", ConflictKinds, want)
	}
}

// conflictKindConsts is every ConflictKind constant the package's non-test
// files declare, value to name, read from the source (TestPredicates_Closed's
// method).
func conflictKindConsts(t *testing.T) map[ConflictKind]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[ConflictKind]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range af.Decls {
			gd, isGen := decl.(*ast.GenDecl)
			if !isGen || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				id, named := vs.Type.(*ast.Ident)
				if !named || id.Name != "ConflictKind" || len(vs.Values) != 1 {
					continue
				}
				lit, isLit := vs.Values[0].(*ast.BasicLit)
				if !isLit {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				if prev, dup := out[ConflictKind(v)]; dup {
					t.Errorf("%s and %s both spell %q", prev, vs.Names[0].Name, v)
				}
				out[ConflictKind(v)] = vs.Names[0].Name
			}
		}
	}
	return out
}
