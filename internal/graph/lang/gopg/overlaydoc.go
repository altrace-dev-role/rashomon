package gopg

import (
	"go/ast"
	"sort"
	"strconv"
	"strings"
)

// MaxDocBytes caps an overlay sym's doc, as pgraph caps the index's
// (pgraph/symbols.go MaxDocBytes). pgraph is a separate Go module the main
// module cannot import, so the two rules below mirror its own.
const MaxDocBytes = 160

// MaxOverlayCalls caps the calls attr of an overlay func or method sym
// (PROVISIONAL). Over the cap the first MaxOverlayCalls names in sorted
// order stay and a final "+N" token counts the names cut.
const MaxOverlayCalls = 32

// builtinCall reports whether name is one of Go's predeclared functions: a
// call to one names no declaration of the program. A switch, not a map: a
// package-level map literal is built by init code on every process start
// (984 B in the hook binary once the overlay made gopg reachable, inittrace),
// and the hook pays every byte of init (TestRES2_InitBudget).
// TestGopg_BuiltinCallIsTheUniverse pins the set to go/types' Universe.
func builtinCall(name string) bool {
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy",
		"delete", "imag", "len", "make", "max", "min",
		"new", "panic", "print", "println", "real", "recover":
		return true
	}
	return false
}

// declDoc is the doc comment text pgraph's docIndex attributes to a
// declaration (pgraph/symbols.go docIndex, lines 97-130, go/doc's rule): a
// function or method takes its own doc; a type, const or var spec takes its
// own, else the declaration's when the declaration holds exactly one spec.
// own is the declaration's or spec's own comment; group is the GenDecl's
// (nil for a function); nspecs is the GenDecl's spec count.
func declDoc(own, group *ast.CommentGroup, nspecs int) string {
	if own != nil {
		return own.Text()
	}
	if group != nil && nspecs == 1 {
		return group.Text()
	}
	return ""
}

// synopsis is the index's doc from a doc comment's text (pgraph/symbols.go
// synopsis, lines 138-157): go/doc's first sentence (docSynopsis, which
// mirrors it without the import), runes 0x20..0x7e only, trimmed, then cut to
// MaxDocBytes and back to the last space.
func synopsis(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range docSynopsis(text) {
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

// callNames is the attr value of the short names body calls: sorted,
// unique, comma-joined, builtins dropped, closures included, capped at
// MaxOverlayCalls with a "+N" tail. "" for no body or no calls.
func callNames(body *ast.BlockStmt) string {
	if body == nil {
		return ""
	}
	set := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if name := calleeName(c.Fun); name != "" && !builtinCall(name) {
				set[name] = true
			}
		}
		return true
	})
	if len(set) == 0 {
		return ""
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	if cut := len(names) - MaxOverlayCalls; cut > 0 {
		names = append(names[:MaxOverlayCalls], "+"+strconv.Itoa(cut))
	}
	return strings.Join(names, ",")
}

// calleeName is the short name a call's Fun expression names: an
// identifier's name or a selector's Sel name, through parentheses and a
// generic instantiation; "" for any other expression (a function literal,
// a call result, an index of a map).
func calleeName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			return x.Sel.Name
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		default:
			return ""
		}
	}
}
