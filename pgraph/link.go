package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/types"
	"hash/fnv"
	"sort"
	"strings"
)

// MinLinkNodes is the body-complexity floor (PF-PL10): a function whose body
// has fewer AST nodes than this is never linked, by rule 3 or rule 4, because
// a stub's body is the same as every other stub's and "the same body" says
// nothing about who it is. Twenty nodes is more than a getter or a one-line
// forwarder (about a dozen) and fewer than a loop with a branch in it (thirty
// and up); a body of six `_ = 1` statements has 19 and one of seven has 22.
// The floor decides only renames and edits: a symbol that keeps its id keeps
// its identity at any size.
//
// Measured on this repository's history under the identity scheme with
// fingerprints: origin/main at 323e4075, its 84 first-parent commits, 78
// consecutive pairs (the oldest five commits import modernc.org/sqlite, which
// the offline loader cannot resolve, so five pairs are out; 56,412 same-id
// links across the 78). With no floor rule 3 had 7 candidates (a symbol gone
// and one of the same shape and body new, unique in both snapshots), none
// ambiguous, and rule 4 had none: this history has no init or blank function
// outside test files. Each of the 7 was read against its commit: all are true
// renames or moves (byTool to byName, buildSilentFailures and sessionWindow
// capitalised, timeoutFor to TimeoutFor, and baselineErrorReason,
// callsWithoutSessionID and subagentTranscripts from internal/report to
// internal/report/gather, bodies unchanged).
//
//	floor   rule-3 links   missed (true, not linked)        false links
//	  10          7        0                                     0
//	  20          6        1  timeoutFor (18 nodes)              0
//	  40          4        3  timeoutFor (18), baselineErrorReason (28),
//	                          callsWithoutSessionID (34)         0
//
// On missed links the data prefers 10 and rejects 40. On false links it cannot
// speak: the smallest of the 7 candidates has 18 nodes, so floors 10 and 0
// behave the same here, and no floor refused anything. What 10 would admit
// that 20 refuses is exactly what the floor exists for, a getter or forwarder
// of a dozen nodes that some other stub equals, and this history has no such
// pair to show it. A false link merges two symbols' histories and a missed link
// costs one retraction and one assertion, so the floor stays at the lowest
// value that keeps clear of that range and has a measured cost of one link: 20.
// The one-character change to 10 would have linked timeoutFor and nothing
// else.
const MinLinkNodes = 20

// LinkSimilarityPercent is the least similarity, in percent, at which Link's
// rule 4 takes two repeatable symbols of one declared name for one symbol
// edited. The Dice coefficient of two bodies is twice the nodes they share over
// the nodes they have, so 90 admits a body that kept nine nodes in ten. On the
// bodies of TestPL10_SimilarityOnRealBodies (an init of 32 nodes): one literal
// edited is 96; the same body with every literal changed (the shape of another
// init of the testdata, filling a registry from 9, 8, 7 where this one fills it
// from 1, 2, 3) is 81, and unrelated bodies are 21 to 38. A shape kept and its
// literals replaced is the one case content cannot call: 90 reads it as a
// different symbol, so an init rewritten that far gets a new id instead of a
// carried one, which costs a retraction and an assertion and never a false
// merge.
const LinkSimilarityPercent = 90

// Fingerprint is what Link knows of one declared function or method at one
// commit: enough to tell a rename from a different function.
type Fingerprint struct {
	// ID is the symbol id (identity.go).
	ID string
	// Repeatable says the name repeats (init, the blank identifier) and the
	// id therefore ends in a fingerprint of the body: an id of this kind says
	// what a symbol is, so an edit to the body is a new id.
	Repeatable bool
	// Declared is the id without its fingerprint (func:PKG.init); equal to ID
	// when the symbol is not repeatable.
	Declared string
	// Shape is the kind and the signature without name or receiver.
	Shape string
	// Body is the hex SHA-256 of the canonical body: the syntax tree with
	// names, literals and operators and without positions, comments, layout,
	// or the function's own name where it refers to itself.
	Body string
	// Nodes is the number of syntax nodes in the body.
	Nodes int
	// Tokens is the body as a multiset, sorted: one hash per syntax node of
	// what the body hash sees of it (its type, and its name, literal or
	// operator). It is kept for repeatable symbols only, for similarity.
	Tokens []uint32
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
				rep := nm.suffix[fn] != ""
				body, nodes, toks := normalize(pkg.TypesInfo, fn, rep, fd.Body)
				id := nm.id(fn)
				fp := Fingerprint{ID: id, Repeatable: rep, Declared: id, Shape: shapeOf(fn), Body: body, Nodes: nodes, Tokens: toks}
				if rep {
					fp.Declared = strings.TrimSuffix(id, nm.suffix[fn])
				}
				out = append(out, fp)
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

// normalize hashes the syntax trees of nodes, in order: each node's Go type and
// bracketing, plus what the type alone does not say (identifier names,
// literals, operators). self is the declaration being hashed: a use of it
// inside its own tree (recursion) hashes as a placeholder, so renaming a
// recursive function is still a pure rename. It returns the hex SHA-256, the
// number of nodes, and, when tokens is set, the sorted multiset of per-node
// token hashes that similarity compares.
func normalize(info *types.Info, self types.Object, tokens bool, nodes ...ast.Node) (string, int, []uint32) {
	h := sha256.New()
	count := 0
	var toks []uint32
	for _, root := range nodes {
		ast.Inspect(root, func(x ast.Node) bool {
			if x == nil {
				fmt.Fprint(h, ")")
				return true
			}
			count++
			tok := fmt.Sprintf("%T", x)
			switch x := x.(type) {
			case *ast.Ident:
				if self != nil && info.Uses[x] == self {
					tok += " self"
				} else {
					tok += fmt.Sprintf(" %q", x.Name)
				}
			case *ast.BasicLit:
				tok += fmt.Sprintf(" %v %q", x.Kind, x.Value)
			case *ast.BinaryExpr:
				tok += fmt.Sprintf(" %v", x.Op)
			case *ast.UnaryExpr:
				tok += fmt.Sprintf(" %v", x.Op)
			case *ast.AssignStmt:
				tok += fmt.Sprintf(" %v", x.Tok)
			case *ast.IncDecStmt:
				tok += fmt.Sprintf(" %v", x.Tok)
			case *ast.BranchStmt:
				tok += fmt.Sprintf(" %v", x.Tok)
			case *ast.RangeStmt:
				tok += fmt.Sprintf(" %v", x.Tok)
			case *ast.GenDecl:
				tok += fmt.Sprintf(" %v", x.Tok)
			case *ast.ChanType:
				tok += fmt.Sprintf(" %v", x.Dir)
			}
			fmt.Fprint(h, "("+tok)
			if tokens {
				t := fnv.New32a()
				t.Write([]byte(tok))
				toks = append(toks, t.Sum32())
			}
			return true
		})
	}
	sort.Slice(toks, func(i, j int) bool { return toks[i] < toks[j] })
	return hex.EncodeToString(h.Sum(nil)), count, toks
}

// similarity is the Dice coefficient of two sorted token multisets: twice the
// size of their multiset intersection over the sum of their sizes, returned as
// that numerator (twice the common count) and denominator so that a threshold
// is compared in integers. Empty against empty is 0 over 0, which no threshold
// admits.
func similarity(a, b []uint32) (num, den int) {
	i, j, common := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			common++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return 2 * common, len(a) + len(b)
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
//     symbol. The id of a repeatable symbol ends in a fingerprint of its shape
//     and body, so this is where an init that only moved keeps its id; the
//     check is kept so that two bodies that agree in the fingerprint's 12
//     digits are not taken for one.
//  3. Of what is left, a symbol links to another when their shape and body are
//     equal, the pair is unique (no other function in either snapshot has
//     that shape and body, so a copy is never a rename), and both bodies reach
//     MinLinkNodes. This is a pure rename or a move, of a function or between
//     receivers.
//  4. Of the repeatable symbols still left, one links to another by best
//     match: both have the same declared name (func:PKG.init) and the same
//     shape, both bodies reach MinLinkNodes, their bodies are at least
//     LinkSimilarityPercent alike, and each is the other's unique best match
//     among the candidates of that name (no tie on either side). This is an
//     edited init, or an edited blank function, carried to its new id.
//     Similarity is the Dice coefficient of the bodies' token multisets (see
//     similarity): twice the nodes they share, over the nodes they have, where
//     a node is shared when its type and its name, literal or operator agree.
//     Position in the file, and in the order of the inits, play no part.
//
// A body that mentions a renamed function by name is a different body, so a
// caller of a renamed function is not itself a pure rename: rule 3 links what
// changed in one place only.
//
// Everything else stays unlinked: an ambiguous match is not guessed, a stub
// is not matched, and a renamed type's methods are linked only when rule 3
// holds for each. A name that is not repeatable has nothing to match by
// similarity: it is the same symbol or it is not, by rule 1. Types, variables
// and constants have no body to compare and are not linked; a local type's id
// changes with its own definition and Link does not follow it.
//
// The result is a partial injection and is independent of input order. It is
// an error for a snapshot to repeat an id: that is a breach of the id
// invariant, and a link built on it would mean nothing. Time is O(n log n)
// plus, for rule 4, the square of the candidates sharing one declared name.
func Link(old, cur []Fingerprint) ([]LinkPair, error) {
	return link(old, cur, MinLinkNodes)
}

// link is Link with the floor as a parameter, so that the floor can be
// measured at other values than the one chosen.
func link(old, cur []Fingerprint, floor int) ([]LinkPair, error) {
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
		if len(os) == 1 && len(ns) == 1 && oc[k] == 1 && nc[k] == 1 && os[0].Nodes >= floor && ns[0].Nodes >= floor {
			link(os[0].ID, ns[0].ID)
		}
	}

	// Rule 4, over what is still unlinked.
	type nameShape struct{ declared, shape string }
	rest := func(fs []Fingerprint, linked map[string]bool) map[nameShape][]Fingerprint {
		m := map[nameShape][]Fingerprint{}
		for _, f := range fs {
			if f.Repeatable && !linked[f.ID] && f.Nodes >= floor {
				k := nameShape{f.Declared, f.Shape}
				m[k] = append(m[k], f)
			}
		}
		return m
	}
	ro, rn := rest(old, linkedOld), rest(cur, linkedNew)
	for k, os := range ro {
		ns := rn[k]
		if len(ns) == 0 {
			continue
		}
		bestNew := make([]int, len(os)) // index into ns of each old's unique best, or -1
		bestOld := make([]int, len(ns))
		for i := range bestNew {
			bestNew[i] = -1
		}
		for j := range bestOld {
			bestOld[j] = -1
		}
		type score struct{ num, den int }
		better := func(a, b score) bool { return a.num*b.den > b.num*a.den }
		sim := make([][]score, len(os))
		for i, o := range os {
			sim[i] = make([]score, len(ns))
			for j, n := range ns {
				sim[i][j].num, sim[i][j].den = similarity(o.Tokens, n.Tokens)
			}
		}
		// unique returns the index of the strictly greatest score, or -1 if
		// the greatest is shared.
		unique := func(n int, at func(int) score) int {
			best, tie := -1, false
			for x := 0; x < n; x++ {
				switch {
				case best < 0 || better(at(x), at(best)):
					best, tie = x, false
				case !better(at(best), at(x)):
					tie = true
				}
			}
			if tie {
				return -1
			}
			return best
		}
		for i := range os {
			bestNew[i] = unique(len(ns), func(j int) score { return sim[i][j] })
		}
		for j := range ns {
			bestOld[j] = unique(len(os), func(i int) score { return sim[i][j] })
		}
		for i := range os {
			j := bestNew[i]
			if j < 0 || bestOld[j] != i {
				continue
			}
			if s := sim[i][j]; s.den > 0 && s.num*100 >= LinkSimilarityPercent*s.den {
				link(os[i].ID, ns[j].ID)
			}
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
