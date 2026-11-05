package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/doc"
	"go/types"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// The symbols sidecar places every declared symbol: beside the log (what the
// code is) and the digests (which bytes it was read from), it says where each
// symbol is and what its doc comment says, so a reader can show a symbol to a
// person or match a task's words against it without loading the code again.
// It is chained to the digests it was written beside, the way the walk file
// chains the log and the digests: a symbols file whose digests line is not
// the sha256 of the digests file next to it is another sync's and reads as
// nothing.
const (
	// SymbolsHeader is the first line of SymbolsFile; the second is
	// "digests\t<sha256 hex of the digests file's bytes>", then one row per
	// symbol in id order: id, package path, file, line, doc, tab-separated.
	// File and doc may be empty (unplaced; no doc comment); the other fields
	// never are. A change of line meaning takes a new header.
	SymbolsHeader = "pgraph-symbols v0"
	// MaxDocBytes caps a symbol's Doc: the first sentence of its doc comment
	// is cut back to a word boundary within it.
	MaxDocBytes = 160
)

// Symbol is one declared symbol's place: the id Extract declares (identity.go),
// the package path the id spells, the file that declares it as a slash path
// relative to the Program's scan root ("" when the declaration is outside the
// root), the line of its name, and Doc, the first sentence of its doc comment
// by go/doc's synopsis rule, folded to printable ASCII (other runes are
// dropped) and cut at a word boundary within MaxDocBytes; "" with no comment.
type Symbol struct {
	ID, Pkg, File string
	Line          int
	Doc           string
}

// Symbols places every object Extract declares in p: package-scope objects,
// methods of named types and init functions, each once, in ascending id
// order. Time is O(source size) for the doc index plus O(n log n) to sort.
func Symbols(p *Program) []Symbol {
	nm := newNamer(p)
	var out []Symbol
	seen := map[types.Object]bool{}
	for _, pkg := range p.pkgs {
		docs := docIndex(pkg)
		place := func(obj types.Object) {
			if obj == nil || seen[obj] {
				return
			}
			seen[obj] = true
			pos := p.fset.Position(obj.Pos())
			rel, err := normaliseUnder(p.root, pos.Filename)
			if err != nil {
				rel = ""
			}
			out = append(out, Symbol{ID: nm.id(obj), Pkg: obj.Pkg().Path(), File: rel, Line: pos.Line, Doc: synopsis(docs[obj])})
		}
		sc := pkg.Types.Scope()
		for _, name := range sc.Names() {
			obj := sc.Lookup(name)
			place(obj)
			if tn, ok := obj.(*types.TypeName); ok && !tn.IsAlias() {
				if n, ok := tn.Type().(*types.Named); ok {
					for i := 0; i < n.NumMethods(); i++ {
						place(n.Method(i))
					}
				}
			}
		}
		for _, obj := range pkg.TypesInfo.Defs {
			if fn, ok := obj.(*types.Func); ok && fn.Name() == "init" && fn.Type().(*types.Signature).Recv() == nil {
				place(fn)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// docIndex maps each declared object of pkg to its doc comment's text: a
// function's or method's own; a type, const or var spec's own, else the
// declaration keyword's when the declaration holds that one spec (the usual
// "// T is ..." above "type T struct"), as go/doc attributes them.
func docIndex(pkg *packages.Package) map[types.Object]string {
	docs := map[types.Object]string{}
	put := func(name *ast.Ident, own, group *ast.CommentGroup) {
		obj := pkg.TypesInfo.Defs[name]
		if obj == nil {
			return
		}
		if own != nil {
			docs[obj] = own.Text()
		} else if group != nil {
			docs[obj] = group.Text()
		}
	}
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				put(d.Name, d.Doc, nil)
			case *ast.GenDecl:
				var group *ast.CommentGroup
				if len(d.Specs) == 1 {
					group = d.Doc
				}
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						put(s.Name, s.Doc, group)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							put(n, s.Doc, group)
						}
					}
				}
			}
		}
	}
	return docs
}

// synopsis is Symbol.Doc from a doc comment's text: go/doc's first sentence,
// folded to printable ASCII and cut at a word boundary within MaxDocBytes.
func synopsis(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range new(doc.Package).Synopsis(text) {
		if r >= 0x20 && r < 0x7f {
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	if len(s) > MaxDocBytes {
		s = s[:MaxDocBytes]
		if i := strings.LastIndexByte(s, ' '); i > 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// EncodeSymbols renders the symbols file for progs synced under root, as
// EncodeDigests renders the digests file: SymbolsHeader, the digests line
// naming the sha256 of digests (the bytes EncodeDigests returned for the
// same progs), then every Symbol of every Program with its File re-rooted
// at root, in id order. A Program outside root is an error; a field that
// checkField refuses is an error, so a reader can split every row on tabs.
func EncodeSymbols(root string, progs []*Program, digests []byte) ([]byte, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	var syms []Symbol
	for _, p := range progs {
		prefix, err := rootPrefix(real, p.root)
		if err != nil {
			return nil, err
		}
		for _, s := range Symbols(p) {
			if s.File != "" && prefix != "" {
				s.File = path.Join(prefix, s.File)
			}
			syms = append(syms, s)
		}
	}
	sort.SliceStable(syms, func(i, j int) bool { return syms[i].ID < syms[j].ID })
	sum := sha256.Sum256(digests)
	var b strings.Builder
	b.WriteString(SymbolsHeader + "\ndigests\t" + hex.EncodeToString(sum[:]) + "\n")
	for _, s := range syms {
		for what, f := range map[string]string{"symbol id": s.ID, "symbol package": s.Pkg} {
			if err := checkField(what, f); err != nil {
				return nil, err
			}
		}
		for what, f := range map[string]string{"symbol file": s.File, "symbol doc": s.Doc} {
			if f != "" {
				if err := checkField(what, f); err != nil {
					return nil, err
				}
			}
		}
		b.WriteString(s.ID + "\t" + s.Pkg + "\t" + s.File + "\t" + strconv.Itoa(s.Line) + "\t" + s.Doc + "\n")
	}
	return []byte(b.String()), nil
}

// rootPrefix is the slash path of a Program's scan root under the sync root
// ("" when they are one), both absolute with symlinks resolved.
func rootPrefix(real, progRoot string) (string, error) {
	if progRoot == real {
		return "", nil
	}
	return normaliseUnder(real, progRoot)
}
