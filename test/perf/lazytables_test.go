package perf

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"testing"
)

// lazyGetters are the keyed tables PERF-INIT1 took out of initialisation
// (D123), report's and spend's among them for the margin (no hook, post,
// probe or recap path reads them): each is a getter over a zero-initialised
// struct{ sync.Once; v T } holder named <getter>Once, so the table is built
// by the first process that reads it and by no other.
var lazyGetters = map[string][]string{
	"internal/recap":         {"titles", "whens", "noteKeys"},
	"internal/report":        {"families"},
	"internal/spend":         {"table"},
	"internal/steer":         {"budgetKeys", "mirrorVerifiedOn", "outputKeys"},
	"internal/steer/pattern": {"kinds", "priority"},
	"internal/steer/render":  {"templates"},
	"internal/store":         {"runKindNames"},
	"internal/verify":        {"markerTable", "obligationScopes", "integrityKinds", "claimKinds", "reasonState"},
}

// staticWordSets are the membership tables PERF-INIT1 made slices of
// constants, which the compiler lays out in the binary with no code run at
// init; baseline's and report's are on no hook, post, probe or recap path.
var staticWordSets = map[string][]string{
	"internal/baseline": {"ubiquitous"},
	"internal/recap":    {"prefixWords"},
	"internal/report":   {"clientPlaneHosts", "inputCompletedByHarness"},
	"internal/verify": {"noWrite", "generatedSegments", "generatedRootSegments", "ciSegments", "ciBasenames",
		"testSegments", "camelTestExtensions", "docsRootSegments", "docsExtensions", "docsBasenames",
		"configExtensions", "configBasenames"},
}

// TestRES2f_SubcommandTablesAreLazy: no table PERF-INIT1 moved returns to
// init (RES-2). A getter is a function whose first statement is its holder's
// Do, with no package-level var of the getter's name and a holder with no
// initialiser; a word set is a []string literal of string literals and
// nothing else, so no call, conversion or map is built for it at init.
func TestRES2f_SubcommandTablesAreLazy(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	for dir, names := range lazyGetters {
		funcs, vars := topLevel(parseNonTest(t, fset, filepath.Join(root, dir)))
		for _, name := range names {
			if v, ok := vars[name]; ok {
				t.Errorf("%s: %s is a package-level var at %s, want a getter", dir, name, fset.Position(v.Pos()))
			}
			holder, ok := vars[name+"Once"]
			switch {
			case !ok:
				t.Errorf("%s: no holder var %sOnce", dir, name)
			case len(holder.Values) != 0:
				t.Errorf("%s: %sOnce has an initialiser at %s, which runs at init", dir, name, fset.Position(holder.Pos()))
			}
			f, ok := funcs[name]
			if !ok || len(f.Body.List) == 0 || !callsDo(f.Body.List[0], name+"Once") {
				t.Errorf("%s: %s does not open with %sOnce.Do", dir, name, name)
			}
		}
	}
	for dir, names := range staticWordSets {
		_, vars := topLevel(parseNonTest(t, fset, filepath.Join(root, dir)))
		for _, name := range names {
			v, ok := vars[name]
			if !ok || len(v.Values) != 1 || !stringConstants(v.Values[0]) {
				t.Errorf("%s: %s is not a []string literal of string literals", dir, name)
			}
		}
	}
}

// topLevel indexes files' package-level functions and vars by name; a var
// spec that declares several names is indexed under each.
func topLevel(files []*ast.File) (map[string]*ast.FuncDecl, map[string]*ast.ValueSpec) {
	funcs, vars := map[string]*ast.FuncDecl{}, map[string]*ast.ValueSpec{}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					funcs[d.Name.Name] = d
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, s := range d.Specs {
					vs := s.(*ast.ValueSpec)
					for _, n := range vs.Names {
						vars[n.Name] = vs
					}
				}
			}
		}
	}
	return funcs, vars
}

// callsDo reports whether s is the statement <holder>.Do(...).
func callsDo(s ast.Stmt, holder string) bool {
	es, ok := s.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Do" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == holder
}

// stringConstants reports whether e is []string{...} with every element a
// string literal.
func stringConstants(e ast.Expr) bool {
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return false
	}
	at, ok := lit.Type.(*ast.ArrayType)
	if !ok || at.Len != nil {
		return false
	}
	if id, ok := at.Elt.(*ast.Ident); !ok || id.Name != "string" {
		return false
	}
	for _, el := range lit.Elts {
		if b, ok := el.(*ast.BasicLit); !ok || b.Kind != token.STRING {
			return false
		}
	}
	return true
}
