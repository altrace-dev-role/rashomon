package pgraph

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"testing"

	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
)

// rederive computes the same four predicates by routes that share no
// traversal with extract.go: the extractor walks declarations and statement
// bodies with a stack; this walks by call-expression and resolves the caller
// by source interval, finds declarations from the AST rather than the scope,
// reads embeds from the scope rather than the AST, and checks satisfaction by
// comparing method sets name by name rather than calling types.Implements.
// Only the naming of a symbol (symbolID) is shared.
func rederive(t *testing.T, p *Program) []Triple {
	t.Helper()
	var out []Triple
	var concretes, ifaces []*types.Named
	for _, pkg := range p.pkgs {
		out = append(out, oracleCalls(pkg)...)
		out = append(out, oracleDecls(pkg)...)
		out = append(out, oracleEmbeds(pkg)...)
		for _, obj := range pkg.TypesInfo.Defs {
			tn, ok := obj.(*types.TypeName)
			if !ok || tn.IsAlias() || tn.Parent() != pkg.Types.Scope() {
				continue
			}
			n, ok := tn.Type().(*types.Named)
			if !ok || n.TypeParams().Len() != 0 {
				continue
			}
			if it, ok := n.Underlying().(*types.Interface); ok {
				if it.IsMethodSet() && it.NumMethods() > 0 {
					ifaces = append(ifaces, n)
				}
			} else {
				concretes = append(concretes, n)
			}
		}
	}
	for _, c := range concretes {
		for _, i := range ifaces {
			if oracleSatisfies(types.NewMethodSet(c), i) {
				out = append(out, Triple{symbolID(c.Obj(), p.fset), Implements, symbolID(i.Obj(), p.fset)})
			} else if oracleSatisfies(types.NewMethodSet(types.NewPointer(c)), i) {
				out = append(out, Triple{symbolID(c.Obj(), p.fset), ImplementsViaPointer, symbolID(i.Obj(), p.fset)})
			}
		}
	}
	sort.Slice(out, func(a, b int) bool { return tripleLess(out[a], out[b]) })
	return out
}

func oracleSatisfies(ms *types.MethodSet, iface *types.Named) bool {
	it := iface.Underlying().(*types.Interface)
	for i := 0; i < it.NumMethods(); i++ {
		want := it.Method(i)
		sel := ms.Lookup(want.Pkg(), want.Name())
		if sel == nil || !types.Identical(sel.Type(), want.Type()) {
			return false
		}
	}
	return true
}

type span struct {
	lo, hi token.Pos
	id     string
}

func oracleCalls(pkg *packages.Package) []Triple {
	var spans []span
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				if fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func); ok {
					spans = append(spans, span{fd.Pos(), fd.End(), symbolID(fn, pkg.Fset)})
				}
			}
		}
	}
	callerAt := func(pos token.Pos) string {
		for _, s := range spans {
			if s.lo <= pos && pos < s.hi {
				return s.id
			}
		}
		return "pkg:" + pkg.PkgPath
	}
	var out []Triple
	inspector.New(pkg.Syntax).Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if pkg.TypesInfo.Types[call.Fun].IsType() {
			return
		}
		var obj types.Object
		fun := call.Fun
		for obj == nil {
			switch e := fun.(type) {
			case *ast.ParenExpr:
				fun = e.X
			case *ast.IndexExpr:
				fun = e.X
			case *ast.IndexListExpr:
				fun = e.X
			case *ast.Ident:
				obj = pkg.TypesInfo.Uses[e]
				if obj == nil {
					return
				}
			case *ast.SelectorExpr:
				if sel := pkg.TypesInfo.Selections[e]; sel != nil {
					obj = sel.Obj()
				} else if obj = pkg.TypesInfo.Uses[e.Sel]; obj == nil {
					return
				}
			default:
				return
			}
		}
		if fn, ok := obj.(*types.Func); ok {
			out = append(out, Triple{callerAt(call.Pos()), Calls, symbolID(fn.Origin(), pkg.Fset)})
		}
	})
	return out
}

func oracleDecls(pkg *packages.Package) []Triple {
	pk := "pkg:" + pkg.PkgPath
	var out []Triple
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, s := range d.Specs {
					var names []*ast.Ident
					switch s := s.(type) {
					case *ast.TypeSpec:
						names = []*ast.Ident{s.Name}
					case *ast.ValueSpec:
						names = s.Names
					}
					for _, id := range names {
						if obj := pkg.TypesInfo.Defs[id]; obj != nil && obj.Parent() == pkg.Types.Scope() {
							out = append(out, Triple{pk, Declares, symbolID(obj, pkg.Fset)})
						}
					}
				}
			case *ast.FuncDecl:
				fn, ok := pkg.TypesInfo.Defs[d.Name].(*types.Func)
				if !ok {
					continue
				}
				if d.Recv == nil {
					out = append(out, Triple{pk, Declares, symbolID(fn, pkg.Fset)})
					continue
				}
				rt := fn.Type().(*types.Signature).Recv().Type()
				if ptr, ok := rt.(*types.Pointer); ok {
					rt = ptr.Elem()
				}
				if n, ok := types.Unalias(rt).(*types.Named); ok {
					out = append(out, Triple{symbolID(n.Obj(), pkg.Fset), Declares, symbolID(fn, pkg.Fset)})
				}
			}
		}
	}
	return out
}

func oracleEmbeds(pkg *packages.Package) []Triple {
	var out []Triple
	sc := pkg.Types.Scope()
	for _, name := range sc.Names() {
		tn, ok := sc.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		n, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		var embedded []types.Type
		switch u := n.Underlying().(type) {
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if u.Field(i).Embedded() {
					embedded = append(embedded, u.Field(i).Type())
				}
			}
		case *types.Interface:
			for i := 0; i < u.NumEmbeddeds(); i++ {
				embedded = append(embedded, u.EmbeddedType(i))
			}
		}
		for _, e := range embedded {
			e = types.Unalias(e)
			if ptr, ok := e.(*types.Pointer); ok {
				e = types.Unalias(ptr.Elem())
			}
			if en, ok := e.(*types.Named); ok && en.Obj().Pkg() != nil {
				out = append(out, Triple{symbolID(tn, pkg.Fset), Embeds, symbolID(en.Obj(), pkg.Fset)})
			}
		}
	}
	return out
}
