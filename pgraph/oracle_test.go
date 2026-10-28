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
// Only the naming of a symbol (namer.id) is shared.
func rederive(t *testing.T, p *Program) []Triple {
	t.Helper()
	var out []Triple
	nm := newNamer(p)
	var concretes, ifaces []*types.Named
	for _, pkg := range p.pkgs {
		out = append(out, oracleCalls(pkg, nm)...)
		out = append(out, oracleDecls(pkg, nm)...)
		out = append(out, oracleEmbeds(pkg, nm)...)
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
				out = append(out, Triple{nm.id(c.Obj()), Implements, nm.id(i.Obj())})
			} else if oracleSatisfies(types.NewMethodSet(types.NewPointer(c)), i) {
				out = append(out, Triple{nm.id(c.Obj()), ImplementsViaPointer, nm.id(i.Obj())})
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

func oracleCalls(pkg *packages.Package, nm *namer) []Triple {
	var spans []span
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				if fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func); ok {
					spans = append(spans, span{fd.Pos(), fd.End(), nm.id(fn)})
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
			out = append(out, Triple{callerAt(call.Pos()), Calls, nm.id(fn.Origin())})
		}
	})
	return out
}

func oracleDecls(pkg *packages.Package, nm *namer) []Triple {
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
							out = append(out, Triple{pk, Declares, nm.id(obj)})
						}
					}
				}
			case *ast.FuncDecl:
				fn, ok := pkg.TypesInfo.Defs[d.Name].(*types.Func)
				if !ok {
					continue
				}
				if d.Recv == nil {
					out = append(out, Triple{pk, Declares, nm.id(fn)})
					continue
				}
				rt := fn.Type().(*types.Signature).Recv().Type()
				if ptr, ok := rt.(*types.Pointer); ok {
					rt = ptr.Elem()
				}
				if n, ok := types.Unalias(rt).(*types.Named); ok {
					out = append(out, Triple{nm.id(n.Obj()), Declares, nm.id(fn)})
				}
			}
		}
	}
	return out
}

func oracleEmbeds(pkg *packages.Package, nm *namer) []Triple {
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
				out = append(out, Triple{nm.id(tn), Embeds, nm.id(en.Obj())})
			}
		}
	}
	return out
}

// callerResolver maps a source position to the id of the declared function
// whose text holds it, or to the package for a package-level initialiser.
func callerResolver(pkg *packages.Package, nm *namer) func(token.Pos) string {
	var spans []span
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				if fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func); ok {
					spans = append(spans, span{fd.Pos(), fd.End(), nm.id(fn)})
				}
			}
		}
	}
	return func(pos token.Pos) string {
		for _, s := range spans {
			if s.lo <= pos && pos < s.hi {
				return s.id
			}
		}
		return "pkg:" + pkg.PkgPath
	}
}

// rederiveDispatch computes the may-call facts without SSA or go/callgraph:
// for every call on an interface method found in the syntax, the targets are
// the methods that the method set of each concrete named type of the scanned
// packages (or of its pointer) selects, when the type satisfies the interface.
// A method promoted from an embedded interface selects an abstract method,
// which stands for whatever the embedded interface can hold, so the search
// continues on that interface. This is the same algorithm as the extractor's
// by a different route: types.Implements and method sets against SSA and its
// call graph.
func rederiveDispatch(t *testing.T, p *Program) []Triple {
	t.Helper()
	nm := newNamer(p)
	roots := map[string]bool{}
	var concretes []*types.Named
	for _, pkg := range p.pkgs {
		roots[pkg.PkgPath] = true
		sc := pkg.Types.Scope()
		for _, name := range sc.Names() {
			tn, ok := sc.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			if n, ok := tn.Type().(*types.Named); ok && n.TypeParams().Len() == 0 && !types.IsInterface(n) {
				concretes = append(concretes, n)
			}
		}
	}
	type query struct {
		iface *types.Interface
		m     *types.Func
	}
	memo := map[query][]*types.Func{}
	var targets func(q query, onPath map[query]bool) []*types.Func
	targets = func(q query, onPath map[query]bool) []*types.Func {
		if r, ok := memo[q]; ok {
			return r
		}
		if onPath[q] {
			return nil
		}
		onPath[q] = true
		defer delete(onPath, q)
		var out []*types.Func
		for _, c := range concretes {
			for _, T := range []types.Type{c, types.NewPointer(c)} {
				if !types.Implements(T, q.iface) {
					continue
				}
				sel := types.NewMethodSet(T).Lookup(q.m.Pkg(), q.m.Name())
				if sel == nil {
					continue
				}
				f := sel.Obj().(*types.Func)
				if recv := f.Type().(*types.Signature).Recv().Type(); types.IsInterface(recv) {
					out = append(out, targets(query{recv.Underlying().(*types.Interface), f}, onPath)...)
				} else {
					out = append(out, f)
				}
			}
		}
		memo[q] = out
		return out
	}
	var out []Triple
	for _, pkg := range p.pkgs {
		callerAt := callerResolver(pkg, nm)
		inspector.New(pkg.Syntax).Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
			call := n.(*ast.CallExpr)
			fun := call.Fun
			for {
				pe, ok := fun.(*ast.ParenExpr)
				if !ok {
					break
				}
				fun = pe.X
			}
			se, ok := fun.(*ast.SelectorExpr)
			if !ok {
				return
			}
			sel := pkg.TypesInfo.Selections[se]
			if sel == nil || sel.Kind() != types.MethodVal {
				return
			}
			// The type that owns the method after walking the embedded path.
			typ := sel.Recv()
			idx := sel.Index()
			for _, i := range idx[:len(idx)-1] {
				if ptr, ok := typ.Underlying().(*types.Pointer); ok {
					typ = ptr.Elem()
				}
				typ = typ.Underlying().(*types.Struct).Field(i).Type()
			}
			if _, tp := types.Unalias(typ).(*types.TypeParam); tp || !types.IsInterface(typ) {
				return
			}
			m := sel.Obj().(*types.Func)
			for _, f := range targets(query{typ.Underlying().(*types.Interface), m}, map[query]bool{}) {
				if f.Pkg() != nil && roots[f.Pkg().Path()] {
					out = append(out, Triple{callerAt(call.Pos()), MayCall, nm.id(f.Origin())})
				}
			}
		})
	}
	sort.Slice(out, func(a, b int) bool { return tripleLess(out[a], out[b]) })
	uniq := out[:0]
	for i, x := range out {
		if i == 0 || x != out[i-1] {
			uniq = append(uniq, x)
		}
	}
	return uniq
}
