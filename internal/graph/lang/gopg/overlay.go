package gopg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// overlayConf is the confidence of every overlay record: a parse of one
// file's declarations, never a type check, so never PROVEN.
const overlayConf = 0.8

// overlayProv names go/parser as the source of an overlay record.
var overlayProv = graph.Prov{Extractor: "go/parser", Version: "v1"}

// Overlay is the reduced extractor over one file's post-edit content: one
// go/parser pass with object resolution skipped, declarations only (a
// function or method, a type, a const, a var), each a sym node at the
// overlay revision with a Declares edge from the file node, whose digest
// attr is the content's ContentDigest. The ids are the ones Extract gives
// the same declaration, so an overlay replaces a symbol and never
// duplicates it. A sym carries the index's doc (overlaydoc.go: pgraph's
// attribution and synopsis rules) and a func or method the short names its
// body calls (calls attr); no body is kept, and a sig is the function's type
// spelled by its tokens (sigtokens.go), nothing of its statements. The file node is contained by its
// unit (one Contains edge, the index's key). With an ImportMap the file
// also imports each unit its import clause maps to (imports.go: one Imports
// edge per unit, none from a _test.go file), the edges the index gives the
// same file. Content that does not parse yields nil records and a nil
// error; the overlay says less, never wrong. It reads no file and starts no
// process: the map's directory check is its caller's.
func (r Resolver) Overlay(rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, content, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		return nil, nil
	}
	rev := graph.Rev{Kind: graph.RevKindOverlay, Commit: base.Commit, Seq: seq}
	unit := path.Dir(rel)
	file := graph.FileID(rel)
	out := []graph.Record{{Node: &graph.Node{ID: file, Kind: "file", Tier: graph.Algorithmic, Conf: overlayConf, Rev: rev, Prov: overlayProv,
		Attrs: map[string]string{"digest": graph.ContentDigest(content), "lang": Lang}}}}
	if u, ok := (Resolver{}).Unit("", rel); ok {
		out = append(out, graph.Record{Edge: &graph.Edge{Src: u, Pred: graph.Contains, Dst: file, Tier: graph.Algorithmic, Conf: overlayConf, Rev: rev, Prov: overlayProv}})
	}
	seen := map[graph.ID]bool{}
	add := func(kind, qualified, name, sig, docText, calls string) {
		if name == "_" {
			return
		}
		id := graph.SymID(Lang, unit, kind, qualified)
		if seen[id] {
			return
		}
		seen[id] = true
		attrs := map[string]string{"name": name, "symkind": kind}
		if sig != "" {
			attrs["sig"] = sig
		}
		if d := synopsis(docText); d != "" {
			attrs["doc"] = d
		}
		if calls != "" {
			attrs["calls"] = calls
		}
		out = append(out,
			graph.Record{Node: &graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: overlayConf, Rev: rev, Prov: overlayProv, Attrs: attrs}},
			graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: overlayConf, Rev: rev, Prov: overlayProv}})
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			funcDecl(fset, content, d, add)
		case *ast.GenDecl:
			genDecl(d, add)
		}
	}
	if r.Imports != nil {
		out = append(out, fileImports(f, rel, *r.Imports, func(dst graph.ID) graph.Record {
			return graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Imports, Dst: dst, Tier: graph.Algorithmic, Conf: overlayConf, Rev: rev, Prov: overlayProv}}
		})...)
	}
	return out, nil
}

// OverlayCarried is exactly the predicates Overlay emits: Declares, from the
// file node to each declaration, and Imports, from the file, when the
// resolver maps imports (a usable ImportMap). Calls and references are not
// read, nor imports without a map, so an overlay is silent on them and the
// index's edges of those predicates stand.
func (r Resolver) OverlayCarried() graph.Carried {
	if r.mapsImports() {
		return graph.Carried{graph.Declares, graph.Imports}
	}
	return graph.Carried{graph.Declares}
}

// mapsImports is whether the resolver's ImportMap can map an import.
func (r Resolver) mapsImports() bool {
	return r.Imports != nil && r.Imports.Module != "" && r.Imports.IsDir != nil
}

// funcDecl reports a function or a method. A method's qualified spelling is
// Recv.Name with the pointer and the type parameters of the receiver
// dropped, as pgraph spells it.
func funcDecl(fset *token.FileSet, content []byte, d *ast.FuncDecl, add func(kind, qualified, name, sig, docText, calls string)) {
	sig := sigSpelling(fset, content, d.Type)
	name := d.Name.Name
	if d.Recv == nil || len(d.Recv.List) == 0 {
		add("func", name, name, sig, declDoc(d.Doc, nil, 1), callNames(d.Body))
		return
	}
	if recv := recvName(d.Recv.List[0].Type); recv != "" {
		add("method", recv+"."+name, name, sig, declDoc(d.Doc, nil, 1), callNames(d.Body))
	}
}

// recvName is the receiver's type name: T of T, *T, T[P] and *T[P, Q].
func recvName(e ast.Expr) string {
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
			return x.Name
		default:
			return ""
		}
	}
}

// genDecl reports the types, constants and variables of one declaration.
func genDecl(d *ast.GenDecl, add func(kind, qualified, name, sig, docText, calls string)) {
	for _, s := range d.Specs {
		switch s := s.(type) {
		case *ast.TypeSpec:
			add("type", s.Name.Name, s.Name.Name, "", declDoc(s.Doc, d.Doc, len(d.Specs)), "")
		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			for _, n := range s.Names {
				add(kind, n.Name, n.Name, "", declDoc(s.Doc, d.Doc, len(d.Specs)), "")
			}
		}
	}
}
