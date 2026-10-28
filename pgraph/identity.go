package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/token"
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
// position, file name, offset or ordinal is ever part of an id, so moving
// text, inserting a declaration above another, or editing a body elsewhere
// changes no id.
//
// Names that Go lets repeat inside one package, init and the blank
// identifier (as a function or a method) and types declared inside function
// bodies, are told apart by what they are, not where they are: the id is the
// declared identity (func:PKG.init, type:PKG.Outer$name, where a local type's
// spelling includes its enclosing declaration) and "#" and a fingerprint of the
// normalized declaration. The fingerprint is the first 12 hex digits of the
// SHA-256 of the signature and the body's syntax tree with names, literals and
// operators and without positions, comments, layout or the symbol's own name
// where it refers to itself (the same normalization Link compares), so a
// function's fingerprint covers its signature and body, and a local type's the
// type expression and type parameters. A lone init carries one too, so that
// adding a second renames nothing. An enclosing declaration that is itself
// repeatable is spelled with its own fingerprint (init#9f2c4a7e1b3d$T#...),
// so the same local type in two different inits is two different symbols.
//
// Exact duplicates (the same declared identity and the same fingerprint:
// identical bodies) are numbered: the first keeps the bare "#fp" and the rest
// are "#fp~2", "#fp~3". The numbering is a rank among the duplicates only,
// never among siblings, and the duplicates are ordered by what the text can
// say (the full hash, which separates the rare fingerprints that agree in 12
// digits) and, for duplicates that nothing but their place tells apart, by
// source order. Adding, removing or moving any declaration that is not an
// exact duplicate of another changes no id; adding or removing a copy of a
// body changes which text holds which of "#fp", "#fp~2" and not the set of
// ids, and two such copies are interchangeable by construction (the same
// signature and body in the same declared scope, so the facts about one are
// the facts about the other) except for a local type that the code uses
// asymmetrically, which is the one place the rank is observable.
//
// Invariant: within one Program, two distinct declared symbols have two
// distinct ids. Names without a fingerprint are unique by Go's scoping rules
// (one object per name per package scope, one method per name per type);
// names with one are made unique by the fingerprint and the numbering of
// duplicates, which are assigned over the groups of objects whose spelling
// including the fingerprint is the same. The exception is by design: methods
// of identical anonymous interface types are one symbol.
//
// A fingerprint says what a symbol is, so editing its body changes its id.
// That is what Link is for: it carries a repeatable symbol across an edit by
// the best match within its declared name.
type namer struct {
	suffix map[types.Object]string // "#fp" or "#fp~k" of repeatable objects declared in the scan
	owner  map[types.Object]string // local types: the declaration they sit in, spelled
}

// maxAnonReceiverBytes bounds the spelled text of an anonymous interface
// receiver; beyond it the text is replaced by a hash of it, so an id stays far
// under MaxFieldBytes.
const maxAnonReceiverBytes = 200

// fingerprintDigits is the length, in hex digits, of a fingerprint in an id.
const fingerprintDigits = 12

// repeated is one object whose id carries a fingerprint, with what orders the
// objects that share one.
type repeated struct {
	obj  types.Object
	base string // the declared identity
	full string // the full hex SHA-256 the fingerprint is cut from
	pos  token.Position
}

// newNamer scans the Program's syntax once. Time is O(source size) plus
// sorting each group of repeatable objects.
func newNamer(p *Program) *namer {
	n := &namer{suffix: map[types.Object]string{}, owner: map[types.Object]string{}}
	var funcs []repeated
	for _, pkg := range p.pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				if base, ok := n.repeatableBase(fn); ok {
					funcs = append(funcs, repeated{fn, base, bodyDigest(pkg, fd, fn), p.fset.Position(fn.Pos())})
				}
			}
		}
	}
	n.number(funcs)
	var types_ []repeated
	for _, pkg := range p.pkgs {
		for _, o := range n.findLocalTypes(pkg) {
			base := o.tn.Pkg().Path() + "." + o.owner + "$" + o.tn.Name()
			n.owner[o.tn] = o.owner
			types_ = append(types_, repeated{o.tn, "type:" + base, typeDigest(pkg, o.spec, o.tn), p.fset.Position(o.tn.Pos())})
		}
	}
	n.number(types_)
	return n
}

// number gives each object of rs its suffix: the fingerprint, and a rank among
// the objects that share declared identity and fingerprint.
func (n *namer) number(rs []repeated) {
	groups := map[string][]repeated{}
	for _, r := range rs {
		k := r.base + "#" + r.full[:fingerprintDigits]
		groups[k] = append(groups[k], r)
	}
	for _, g := range groups {
		sort.Slice(g, func(i, j int) bool {
			if g[i].full != g[j].full {
				return g[i].full < g[j].full
			}
			a, b := g[i].pos, g[j].pos
			if a.Filename != b.Filename {
				return a.Filename < b.Filename
			}
			return a.Offset < b.Offset
		})
		for i, r := range g {
			s := "#" + r.full[:fingerprintDigits]
			if i > 0 {
				s += "~" + strconv.Itoa(i+1)
			}
			n.suffix[r.obj] = s
		}
	}
}

// localType is a type declared inside a function body or a package-level
// variable's initialiser, with the spelling of the declaration it sits in.
type localType struct {
	tn    *types.TypeName
	spec  *ast.TypeSpec
	owner string
}

// findLocalTypes returns every type declared inside a function body or a
// package-level variable's initialiser. The enclosing declaration is spelled
// by its name, and by its own fingerprint when that name repeats (an init, a
// blank function, a blank variable), so that two inits do not share a spelling.
func (n *namer) findLocalTypes(pkg *packages.Package) []localType {
	var out []localType
	collect := func(owner string, root ast.Node) {
		ast.Inspect(root, func(x ast.Node) bool {
			if ts, ok := x.(*ast.TypeSpec); ok {
				if o, ok := pkg.TypesInfo.Defs[ts.Name].(*types.TypeName); ok {
					out = append(out, localType{o, ts, owner})
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
					name := declName(d)
					if fn, ok := pkg.TypesInfo.Defs[d.Name].(*types.Func); ok {
						name += n.suffix[fn]
					}
					collect(name, d.Body)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if vs, ok := s.(*ast.ValueSpec); ok {
						name := vs.Names[0].Name
						if name == "_" {
							h, _, _ := normalize(pkg.TypesInfo, nil, false, valueNodes(vs)...)
							name += "#" + h[:fingerprintDigits]
						}
						for _, v := range vs.Values {
							collect(name, v)
						}
					}
				}
			}
		}
	}
	return out
}

func valueNodes(vs *ast.ValueSpec) []ast.Node {
	out := make([]ast.Node, len(vs.Values))
	for i, v := range vs.Values {
		out[i] = v
	}
	return out
}

// bodyDigest is the full hash a function's fingerprint is cut from: its shape
// (kind and signature without name or receiver) and its normalized body.
func bodyDigest(pkg *packages.Package, fd *ast.FuncDecl, fn *types.Func) string {
	body := ""
	if fd.Body != nil {
		body, _, _ = normalize(pkg.TypesInfo, fn, false, fd.Body)
	}
	sum := sha256.Sum256([]byte(shapeOf(fn) + "\x00" + body))
	return hex.EncodeToString(sum[:])
}

// typeDigest is the full hash a local type's fingerprint is cut from: its type
// parameters and type expression, normalized, and whether it is an alias.
func typeDigest(pkg *packages.Package, ts *ast.TypeSpec, tn *types.TypeName) string {
	var nodes []ast.Node
	if ts.TypeParams != nil {
		nodes = append(nodes, ts.TypeParams)
	}
	nodes = append(nodes, ts.Type)
	h, _, _ := normalize(pkg.TypesInfo, tn, false, nodes...)
	sum := sha256.Sum256([]byte(strconv.FormatBool(ts.Assign.IsValid()) + "\x00" + h))
	return hex.EncodeToString(sum[:])
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

// repeatableBase returns the declared identity, before its fingerprint, of a
// function or method whose name may repeat in its package (init, and the blank
// identifier).
func (n *namer) repeatableBase(fn *types.Func) (string, bool) {
	sig := fn.Type().(*types.Signature)
	switch {
	case sig.Recv() == nil && (fn.Name() == "init" || fn.Name() == "_"):
		return "func:" + qualified(fn), true
	case sig.Recv() != nil && fn.Name() == "_":
		return "method:" + n.methodSpelling(fn), true
	}
	return "", false
}

// typeBase is a type's spelling without its fingerprint.
func (n *namer) typeBase(o *types.TypeName) string {
	if owner, ok := n.owner[o]; ok && o.Pkg() != nil {
		return o.Pkg().Path() + "." + owner + "$" + o.Name()
	}
	return qualified(o)
}

// typeSpelling is a type's spelling with its fingerprint, if it has one.
func (n *namer) typeSpelling(o *types.TypeName) string {
	return n.withSuffix(o, n.typeBase(o))
}

func (n *namer) withSuffix(o types.Object, s string) string {
	return s + n.suffix[o]
}

// methodSpelling is PKG.Type.Name for a method, without its own fingerprint.
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
			return n.withSuffix(o, "method:"+n.methodSpelling(o))
		}
		return n.withSuffix(o, "func:"+qualified(o))
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
