package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/types"
	"sort"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// Symbol ids (PF-PL10).
//
// An id is a kind, a colon, and a spelling built from names only:
//
//	func:PKG.Name             method:PKG.Type.Name       type:PKG.Name
//	var:PKG.Name              const:PKG.Name
//
// where PKG is the import path. A method spells its receiver type without
// pointer or type parameters (Go allows one method of a name per type, so the
// receiver's pointer-ness is not part of who the method is); a method of an
// anonymous interface spells the package and the interface's own text. No
// position, file name or offset is ever part of an id, so moving text or
// editing a body changes no id.
//
// Names that Go lets repeat inside one package, init and the blank
// identifier (as a function or a method) and types declared inside function
// bodies, carry an ordinal: "#k", k counting from 1 in source order among the
// objects with the same spelling (a local type's spelling includes its
// enclosing declaration, as "Outer$name"). The ordinal is always present for
// these, even for a lone init, so that adding a second one renames nothing.
//
// Invariant: within one Program, two distinct declared symbols have two
// distinct ids. Names without an ordinal are unique by Go's scoping rules
// (one object per name per package scope, one method per name per type); names
// with one are made unique by the ordinal. The exception is by design:
// methods of identical anonymous interface types are one symbol.
//
// An ordinal says where a symbol is, not which one it is. It is stable while
// the order of its siblings is, and Link carries identity across a commit when
// the order changed.
type namer struct {
	ordinal map[types.Object]int    // repeatable objects declared in the scan, 1-based
	owner   map[types.Object]string // local types: the declaration they sit in
}

// maxAnonReceiverBytes bounds the spelled text of an anonymous interface
// receiver; beyond it the text is replaced by a hash of it, so an id stays far
// under MaxFieldBytes.
const maxAnonReceiverBytes = 200

// newNamer scans the Program's syntax once. Time is O(source size) plus
// sorting each group of repeatable objects.
func newNamer(p *Program) *namer {
	n := &namer{ordinal: map[types.Object]int{}, owner: map[types.Object]string{}}
	groups := map[string][]types.Object{}
	for _, pkg := range p.pkgs {
		n.findOwners(pkg)
	}
	for _, pkg := range p.pkgs {
		for _, obj := range pkg.TypesInfo.Defs {
			if base, ok := n.repeatableBase(pkg, obj); ok {
				groups[base] = append(groups[base], obj)
			}
		}
	}
	for _, objs := range groups {
		sort.Slice(objs, func(i, j int) bool {
			a, b := p.fset.Position(objs[i].Pos()), p.fset.Position(objs[j].Pos())
			if a.Filename != b.Filename {
				return a.Filename < b.Filename
			}
			return a.Offset < b.Offset
		})
		for i, o := range objs {
			n.ordinal[o] = i + 1
		}
	}
	return n
}

// findOwners records, for every type declared inside a function body or a
// package-level variable's initialiser, the name of that declaration.
func (n *namer) findOwners(pkg *packages.Package) {
	collect := func(owner string, root ast.Node) {
		ast.Inspect(root, func(x ast.Node) bool {
			if ts, ok := x.(*ast.TypeSpec); ok {
				if o := pkg.TypesInfo.Defs[ts.Name]; o != nil {
					n.owner[o] = owner
				}
			}
			return true
		})
	}
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body != nil {
					collect(declName(d), d.Body)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if vs, ok := s.(*ast.ValueSpec); ok {
						for _, v := range vs.Values {
							collect(vs.Names[0].Name, v)
						}
					}
				}
			}
		}
	}
}

// declName is the name a function declaration goes by in an owner spelling:
// Name, or Type.Name for a method, read from the syntax.
func declName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	e := d.Recv.List[0].Type
	for {
		switch x := e.(type) {
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.Ident:
			return x.Name + "." + d.Name.Name
		default:
			return d.Name.Name
		}
	}
}

// repeatableBase returns the spelling, before its ordinal, of an object whose
// name may repeat in its package.
func (n *namer) repeatableBase(pkg *packages.Package, obj types.Object) (string, bool) {
	switch o := obj.(type) {
	case *types.Func:
		sig := o.Type().(*types.Signature)
		switch {
		case sig.Recv() == nil && (o.Name() == "init" || o.Name() == "_"):
			return "func:" + qualified(o), true
		case sig.Recv() != nil && o.Name() == "_":
			return "method:" + n.methodSpelling(o), true
		}
	case *types.TypeName:
		if _, tp := o.Type().(*types.TypeParam); !tp && o.Parent() != nil && o.Parent() != pkg.Types.Scope() {
			return "type:" + n.typeBase(o), true
		}
	}
	return "", false
}

// typeBase is a type's spelling without its ordinal.
func (n *namer) typeBase(o *types.TypeName) string {
	if owner, ok := n.owner[o]; ok && o.Pkg() != nil {
		return o.Pkg().Path() + "." + owner + "$" + o.Name()
	}
	return qualified(o)
}

// typeSpelling is a type's spelling with its ordinal, if it has one.
func (n *namer) typeSpelling(o *types.TypeName) string {
	return n.withOrdinal(o, n.typeBase(o))
}

func (n *namer) withOrdinal(o types.Object, s string) string {
	if k := n.ordinal[o]; k > 0 {
		return s + "#" + strconv.Itoa(k)
	}
	return s
}

// methodSpelling is PKG.Type.Name for a method, without its ordinal.
func (n *namer) methodSpelling(f *types.Func) string {
	t := types.Unalias(f.Type().(*types.Signature).Recv().Type())
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	if nt, ok := t.(*types.Named); ok {
		return n.typeSpelling(nt.Origin().Obj()) + "." + f.Name()
	}
	s := types.TypeString(t, func(p *types.Package) string { return p.Path() })
	if len(s) > maxAnonReceiverBytes {
		sum := sha256.Sum256([]byte(s))
		s = "interface#" + hex.EncodeToString(sum[:8])
	}
	pkg := ""
	if f.Pkg() != nil {
		pkg = f.Pkg().Path() + "."
	}
	return pkg + s + "." + f.Name()
}

// id names an object for use as a subject or object. Objects the scheme does
// not cover (labels, imports, builtins) get the "object:" fallback.
func (n *namer) id(obj types.Object) string {
	switch o := obj.(type) {
	case *types.Func:
		o = o.Origin()
		if o.Type().(*types.Signature).Recv() != nil {
			return n.withOrdinal(o, "method:"+n.methodSpelling(o))
		}
		return n.withOrdinal(o, "func:"+qualified(o))
	case *types.TypeName:
		return "type:" + n.typeSpelling(o)
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
