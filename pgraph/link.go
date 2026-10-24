package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// MinLinkNodes is the body-complexity floor (PF-PL10): a function whose body
// has fewer AST nodes than this is never linked across a rename, because a
// stub's body is the same as every other stub's and "the same body" says
// nothing about who it is. Twenty nodes is more than a getter or a one-line
// forwarder (about a dozen) and fewer than a loop with a branch in it
// (thirty and up); a body of six `_ = 1` statements has 19 and one of seven
// has 22. The floor decides only renames: a symbol that keeps its id keeps
// its identity at any size.
//
// Measured once, on this repository's history: origin/main at 323e4075, its 84
// first-parent commits, 78 consecutive pairs (the oldest five commits import
// modernc.org/sqlite, which the offline loader cannot resolve, so five pairs
// are out). Rule 3 had 7 candidates in all, a vanished symbol and one
// newcomer of the same shape and body, and not one was ambiguous. Read by hand
// every one is a true rename or move, none a false link. Links by floor: 10: 7
// of 7, 15: 7 of 7, 20: 6 of 7, 25: 6 of 7, 30: 5 of 7. At 20 the miss is
// install.timeoutFor to TimeoutFor (18 nodes); at 30 it also loses
// gather.baselineErrorReason (28). The sample is seven renames, so it neither
// argues for moving the floor (no false link at any floor, one 18-node miss at
// 20) nor shows what the floor itself prevents here: the uniqueness test of
// rule 3 refused nothing in this history. 20 stays as the value PF-PL10 chose.
const MinLinkNodes = 20

// Fingerprint is what Link knows of one declared function or method at one
// commit: enough to tell a rename from a different function.
type Fingerprint struct {
	// ID is the symbol id (identity.go).
	ID string
	// Repeatable says the id carries an ordinal: an id of this kind says
	// where a symbol is, not which one it is.
	Repeatable bool
	// Shape is the kind and the signature without name or receiver.
	Shape string
	// Body is the hex SHA-256 of the canonical body: the syntax tree with
	// names, literals and operators and without positions, comments, layout,
	// or the function's own name where it refers to itself.
	Body string
	// Nodes is the number of syntax nodes in the body.
	Nodes int
}

// Fingerprints returns one Fingerprint for every function and method with a
// body in p, sorted by id. Time is O(source size).
func Fingerprints(p *Program) []Fingerprint {
	nm := newNamer(p)
	var out []Fingerprint
	for _, pkg := range p.pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				body, nodes := hashBody(fd.Body, pkg, fn)
				out = append(out, Fingerprint{ID: nm.id(fn), Repeatable: nm.ordinal[fn] > 0, Shape: shapeOf(fn), Body: body, Nodes: nodes})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func shapeOf(fn *types.Func) string {
	sig := fn.Type().(*types.Signature)
	kind := "func"
	if sig.Recv() != nil {
		kind = "method"
	}
	q := func(p *types.Package) string { return p.Path() }
	return fmt.Sprintf("%s %s -> %s tp=%d", kind, types.TypeString(sig.Params(), q), types.TypeString(sig.Results(), q), sig.TypeParams().Len())
}

// hashBody hashes a function body's syntax tree: each node's Go type and
// bracketing, plus what the type alone does not say (identifier names,
// literals, operators). self is the function being hashed: a use of it inside
// its own body (recursion) hashes as a placeholder, so renaming a recursive
// function is still a pure rename.
func hashBody(body *ast.BlockStmt, pkg *packages.Package, self types.Object) (string, int) {
	h := sha256.New()
	nodes := 0
	ast.Inspect(body, func(x ast.Node) bool {
		if x == nil {
			fmt.Fprint(h, ")")
			return true
		}
		nodes++
		fmt.Fprintf(h, "(%T", x)
		switch x := x.(type) {
		case *ast.Ident:
			if pkg.TypesInfo.Uses[x] == self {
				fmt.Fprint(h, " self")
			} else {
				fmt.Fprintf(h, " %q", x.Name)
			}
		case *ast.BasicLit:
			fmt.Fprintf(h, " %v %q", x.Kind, x.Value)
		case *ast.BinaryExpr:
			fmt.Fprintf(h, " %v", x.Op)
		case *ast.UnaryExpr:
			fmt.Fprintf(h, " %v", x.Op)
		case *ast.AssignStmt:
			fmt.Fprintf(h, " %v", x.Tok)
		case *ast.IncDecStmt:
			fmt.Fprintf(h, " %v", x.Tok)
		case *ast.BranchStmt:
			fmt.Fprintf(h, " %v", x.Tok)
		case *ast.RangeStmt:
			fmt.Fprintf(h, " %v", x.Tok)
		case *ast.GenDecl:
			fmt.Fprintf(h, " %v", x.Tok)
		case *ast.ChanType:
			fmt.Fprintf(h, " %v", x.Dir)
		}
		return true
	})
	return hex.EncodeToString(h.Sum(nil)), nodes
}

// LinkPair says the symbol with id From at one commit is the symbol with id
// To at the next.
type LinkPair struct{ From, To string }

type linkKey struct{ shape, body string }

// Link matches the symbols of two commits and returns the pairs it is sure of,
// sorted by From. A symbol absent from the result is unlinked: it is new, or
// gone, or Link could not tell. Rules, in order:
//
//  1. An id present in both snapshots is the same symbol, unless it is
//     repeatable.
//  2. A repeatable id present in both with the same shape and body is the same
//     symbol (the ordinal and the body agree).
//  3. Of what is left, a symbol links to another when their shape and body are
//     equal, the pair is unique (no other function in either snapshot has
//     that shape and body, so a copy is never a rename), and both bodies reach
//     MinLinkNodes. This is a pure rename or a move, of a function or between
//     receivers, and it is how repeatable symbols follow their bodies when
//     their order changes.
//
// A body that mentions a renamed function by name is a different body, so a
// caller of a renamed function is not itself a pure rename: the rule links
// what changed in one place only.
//
// Everything else stays unlinked: an ambiguous match is not guessed, a stub
// is not matched, and a renamed type's methods are linked only when rule 3
// holds for each. Types, variables and constants have no body to compare and
// are not linked.
//
// The result is a partial injection and is independent of input order. It is
// an error for a snapshot to repeat an id: that is a breach of the id
// invariant, and a link built on it would mean nothing. Time is O(n log n).
func Link(old, cur []Fingerprint) ([]LinkPair, error) {
	if _, err := indexByID("old", old); err != nil {
		return nil, err
	}
	curBy, err := indexByID("new", cur)
	if err != nil {
		return nil, err
	}
	count := func(fs []Fingerprint) map[linkKey]int {
		m := map[linkKey]int{}
		for _, f := range fs {
			m[linkKey{f.Shape, f.Body}]++
		}
		return m
	}
	oc, nc := count(old), count(cur)

	var out []LinkPair
	linkedOld, linkedNew := map[string]bool{}, map[string]bool{}
	link := func(from, to string) {
		out = append(out, LinkPair{from, to})
		linkedOld[from], linkedNew[to] = true, true
	}
	for _, o := range old {
		n, ok := curBy[o.ID]
		if !ok || o.Repeatable != n.Repeatable {
			continue
		}
		if !o.Repeatable || (o.Shape == n.Shape && o.Body == n.Body) {
			link(o.ID, o.ID)
		}
	}
	olds, news := map[linkKey][]Fingerprint{}, map[linkKey][]Fingerprint{}
	for _, o := range old {
		if !linkedOld[o.ID] {
			k := linkKey{o.Shape, o.Body}
			olds[k] = append(olds[k], o)
		}
	}
	for _, n := range cur {
		if !linkedNew[n.ID] {
			k := linkKey{n.Shape, n.Body}
			news[k] = append(news[k], n)
		}
	}
	for k, os := range olds {
		ns := news[k]
		if len(os) == 1 && len(ns) == 1 && oc[k] == 1 && nc[k] == 1 && os[0].Nodes >= MinLinkNodes && ns[0].Nodes >= MinLinkNodes {
			link(os[0].ID, ns[0].ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out, nil
}

func indexByID(side string, fs []Fingerprint) (map[string]Fingerprint, error) {
	m := make(map[string]Fingerprint, len(fs))
	for _, f := range fs {
		if _, dup := m[f.ID]; dup {
			return nil, fmt.Errorf("pgraph: %s snapshot repeats id %q", side, f.ID)
		}
		m[f.ID] = f
	}
	return m, nil
}
