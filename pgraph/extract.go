package pgraph

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"

	"golang.org/x/tools/go/types/typeutil"
)

// ExtractorVersion names the rules below. A change to what Extract emits for
// the same source takes a new version.
const ExtractorVersion = "v0"

func gotypesProducer() Producer {
	p, err := newProducer("gotypes", ExtractorVersion, Proven)
	if err != nil {
		panic(err) // constants; unreachable
	}
	return p
}

// Sync extracts the PROVEN facts of p and reconciles the log to them under
// p's config at the given commit. Facts that no longer hold are retracted; the
// resulting view equals the view of a log built from nothing at this commit.
func Sync(l *Log, p *Program, commit string) (Delta, error) {
	return l.Reconcile(gotypesProducer(), p.cfg.String(), commit, Extract(p))
}

// symbolID names an object for use as a subject or object: a kind, a colon and
// a qualified name. Methods use types.Func.FullName, which spells the receiver,
// so a method and a function never collide. Functions that may repeat in one
// package (init, and the blank identifier) carry their file and byte offset.
// The name is unique within one build; carrying it across commits is a later
// slice's problem.
func symbolID(obj types.Object, fset *token.FileSet) string {
	switch o := obj.(type) {
	case *types.Func:
		if o.Type().(*types.Signature).Recv() != nil {
			return "method:" + o.FullName()
		}
		if o.Name() == "init" || o.Name() == "_" {
			pos := fset.Position(o.Pos())
			return fmt.Sprintf("func:%s.%s#%s@%d", o.Pkg().Path(), o.Name(), filepath.Base(pos.Filename), pos.Offset)
		}
		return "func:" + o.FullName()
	case *types.TypeName:
		return "type:" + qualified(o)
	case *types.Var:
		return "var:" + qualified(o)
	case *types.Const:
		return "const:" + qualified(o)
	}
	return "object:" + qualified(obj)
}

func qualified(o types.Object) string {
	if o.Pkg() == nil {
		return o.Name()
	}
	return o.Pkg().Path() + "." + o.Name()
}

// Extract returns the PROVEN facts of p, sorted and without duplicates. All
// four predicates are read from go/types; nothing here is inferred.
//
//   - Declares: package scope objects, methods of declared types, init functions.
//   - Calls: the static callee of every call expression in a function body (a
//     closure's calls belong to its declaring function) or in a package-level
//     variable initialiser (attributed to the package). Calls through func
//     values, conversions and builtins make no edge.
//   - Embeds: for each type declaration (aliases excluded), the named types its
//     underlying struct or interface embeds.
//   - Implements, ImplementsViaPointer: over the named, non-generic types
//     declared in the scanned packages only, so an interface declared outside
//     the scan root (io.Reader) is never an object.
//
// Time is O(source size) for calls, declarations and embeds, and
// O(T x I x m) for satisfaction with T concrete types, I interfaces and m
// methods; sorting adds O(n log n) in facts.
func Extract(p *Program) []Triple {
	var out []Triple
	add := func(s string, pr Predicate, o string) { out = append(out, Triple{s, pr, o}) }
	var concretes, ifaces []*types.Named
	for _, pkg := range p.pkgs {
		pk := "pkg:" + pkg.PkgPath
		info, sc := pkg.TypesInfo, pkg.Types.Scope()
		for _, name := range sc.Names() {
			obj := sc.Lookup(name)
			add(pk, Declares, symbolID(obj, p.fset))
			tn, ok := obj.(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			n, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			for i := 0; i < n.NumMethods(); i++ {
				add(symbolID(tn, p.fset), Declares, symbolID(n.Method(i), p.fset))
			}
			if n.TypeParams().Len() > 0 {
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
		for _, obj := range info.Defs {
			if fn, ok := obj.(*types.Func); ok && fn.Name() == "init" && fn.Type().(*types.Signature).Recv() == nil {
				add(pk, Declares, symbolID(fn, p.fset))
			}
		}
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if fn, ok := info.Defs[d.Name].(*types.Func); ok && d.Body != nil {
						callsIn(d.Body, info, symbolID(fn, p.fset), p.fset, add)
					}
				case *ast.GenDecl:
					for _, s := range d.Specs {
						switch s := s.(type) {
						case *ast.ValueSpec:
							for _, v := range s.Values {
								callsIn(v, info, pk, p.fset, add)
							}
						case *ast.TypeSpec:
							tn, ok := info.Defs[s.Name].(*types.TypeName)
							if !ok || s.Assign.IsValid() {
								continue
							}
							for _, e := range embeddedOf(tn.Type()) {
								add(symbolID(tn, p.fset), Embeds, symbolID(e.Obj(), p.fset))
							}
						}
					}
				}
			}
		}
	}
	for _, c := range concretes {
		for _, i := range ifaces {
			it := i.Underlying().(*types.Interface)
			switch {
			case types.Implements(c, it):
				add(symbolID(c.Obj(), p.fset), Implements, symbolID(i.Obj(), p.fset))
			case types.Implements(types.NewPointer(c), it):
				add(symbolID(c.Obj(), p.fset), ImplementsViaPointer, symbolID(i.Obj(), p.fset))
			}
		}
	}
	sort.Slice(out, func(a, b int) bool { return tripleLess(out[a], out[b]) })
	uniq := out[:0]
	for i, t := range out {
		if i == 0 || t != out[i-1] {
			uniq = append(uniq, t)
		}
	}
	return uniq
}

func callsIn(n ast.Node, info *types.Info, caller string, fset *token.FileSet, add func(string, Predicate, string)) {
	ast.Inspect(n, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			// Callee already names a generic function or method by its
			// origin, so every instantiation is one edge.
			if fn, ok := typeutil.Callee(info, call).(*types.Func); ok {
				add(caller, Calls, symbolID(fn, fset))
			}
		}
		return true
	})
}

// embeddedOf returns the named types t's underlying struct or interface
// embeds, looking through aliases and one pointer. Embedded predeclared types
// (error, comparable) and non-named elements (unions) are skipped.
func embeddedOf(t types.Type) []*types.Named {
	var elems []types.Type
	switch u := t.Underlying().(type) {
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if u.Field(i).Embedded() {
				elems = append(elems, u.Field(i).Type())
			}
		}
	case *types.Interface:
		for i := 0; i < u.NumEmbeddeds(); i++ {
			elems = append(elems, u.EmbeddedType(i))
		}
	}
	var out []*types.Named
	for _, e := range elems {
		e = types.Unalias(e)
		if ptr, ok := e.(*types.Pointer); ok {
			e = types.Unalias(ptr.Elem())
		}
		if n, ok := e.(*types.Named); ok && n.Obj().Pkg() != nil {
			out = append(out, n)
		}
	}
	return out
}
