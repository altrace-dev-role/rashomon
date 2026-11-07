package engine

import (
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The reuse role (PLAN 5.2, W-M1.12 and W-M1.16): the abstractions the
// task should build on, claimed by symbols whose names the request's
// addition and use phrases hold. Split from roles.go at LAW10's file
// ceiling.

// reuse is the abstraction to reuse: the files declaring callable
// symbols (functions, methods, types) whose names or docs hold the
// request's words beyond those that named the target (the unit's own
// path, the target tokens its symbols' names hold and the frame's verbs
// are spent; a doc's word counts when it names code somewhere), among
// the scope rows at or over SeedFloor, never the target file or a
// consumer. A file inside the unit, or one the unit already calls,
// is a claim on any hit; a file elsewhere when a symbol's name holds a
// word of what the request says to use (a use phrase: what the request
// presumes exists) or names at least ReuseFloor of an addition phrase's
// mass among the tokens that name code, as a unit claims a target: a
// word in common with what the request adds is a neighbour, a symbol
// that names most of it is the thing to take up, and a doc that mentions
// the word is neither. A constant or variable adds its hits to its
// file's claim but never carries it; a file's claim is its best symbol's.
// A claim becomes a reuse line only on a stated basis (W-M1.22,
// AC-M1.5): a use-phrase hit, an addition claim, or a call from the
// addition's existing code. A hit inside the unit, or on a symbol the
// unit calls, with none of the three ranks the claim for Explain and is
// no line: a symptom or a question names no thing to take up, and on the
// W-M1.19 set 81 of 117 such lines stood against a '-' truth while the
// basis rule kept 59 of 72 development rows right against 21.
// Files are ranked by their mass on the use phrases (a name hit weighs
// WeightName, a doc hit WeightSummary, as in Scope), then a symbol the
// addition's existing code calls (what names the addition elsewhere,
// read through its call edges: the thing it reuses is the thing to
// reuse), then by their mass on the addition phrases a symbol claims
// (a symbol naming what the request adds, wherever it lives, over a
// neighbour inside the unit), then inside the unit first, then a file one
// of whose hit symbols code outside the unit already calls, then the mass
// of every hit weighed by the share of the name the request's words
// hold, then the path. At
// most MaxReuse; every claim is returned in rank order for Explain.
func reuse(g Reader, c corpus, rows []ScopeRow, r Roles, weights map[string]float64) ([]RoleFile, []ReuseClaim) {
	skip := map[graph.ID]bool{r.File: true}
	for _, rf := range r.Consumers {
		skip[rf.File] = true
	}
	spent := map[string]bool{}
	for _, t := range append(Tokens(r.Path), r.Frame.Verbs...) {
		spent[t] = true
	}
	T := c.named(r.Target)
	for _, id := range r.Targets {
		if i, ok := c.index[id]; ok {
			for _, t := range c.fields[i].Name {
				spent[t] = spent[t] || T[t]
			}
		}
	}
	uses, adds := phraseTokens(r.Frame.Uses), phraseTokens(r.Frame.Additions)
	named := map[string]bool{}
	for t := range weights {
		named[t] = c.nameDF[t] > 0
	}
	found := map[graph.ID]*reuseClaim{}
	var order []graph.ID
	var viaAdd []graph.ID
	for _, row := range rows {
		if row.Score < SeedFloor {
			break
		}
		i, ok := c.index[row.ID]
		if !ok {
			continue
		}
		h := reuseHits(c.fields[i], spent, uses, adds, named, weights)
		claims := h.name && h.claims(c, r.Frame.Additions, weights)
		if claims {
			viaAdd = append(viaAdd, row.ID)
		}
		file := declaring(g, c.rev, row.ID)
		if h.mass == 0 || file == "" || skip[file] {
			continue
		}
		in := c.units[i] == r.Path
		if !in && !usedByUnit(g, c, row.ID, r.Path) && !(h.name && (h.use > 0 || claims)) {
			continue
		}
		if !claims {
			h.add = 0
		}
		cl := found[file]
		if cl == nil {
			cl = &reuseClaim{file: file, in: in}
			found[file] = cl
			order = append(order, file)
		}
		cl.take(row.ID, h, calledOutside(g, c, row.ID, r.Path))
	}
	for _, id := range viaAdd {
		for _, e := range g.Out(id, c.rev, graph.Calls, graph.MayCall, graph.References) {
			if cl := found[declaring(g, c.rev, e.Dst)]; cl != nil && contains(cl.nodes, e.Dst) {
				cl.viaAdd = true
			}
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return found[order[i]].before(*found[order[j]]) })
	var out []RoleFile
	var claims []ReuseClaim
	for _, f := range order {
		cl := found[f]
		claims = append(claims, ReuseClaim{File: f, Use: cl.use, Add: cl.add, Mass: cl.mass, ViaAdd: cl.viaAdd, In: cl.in, Reused: cl.reused, Nodes: len(cl.nodes)})
		if cl.called && cl.basis() && len(out) < MaxReuse {
			out = append(out, RoleFile{File: f, Members: cl.nodes})
		}
	}
	return out, claims
}

// ReuseFloor is the share of an addition phrase's mass, among its tokens
// that name code, one symbol must hold (its head among them) to claim
// the phrase: to be the thing to reuse from outside the unit, or the
// addition's existing code.
const ReuseFloor = 0.5

// reuseClaim is one file's claim on the reuse role while it is built:
// the best of its symbols' masses, whether it is inside the unit, whether
// the addition's existing code calls one of its symbols, whether code
// outside the unit calls one, whether a callable carries it, and its hit
// symbols, callables first.
type reuseClaim struct {
	file                       graph.ID
	use, add, mass             float64
	in, viaAdd, reused, called bool
	nodes                      []graph.ID
}

// basis reports whether the claim can state why its file is the thing
// to reuse: a use-phrase hit, an addition claim, or a call from the
// addition's existing code. Membership of the unit and a call from the
// unit rank a claim and never carry a line.
func (cl reuseClaim) basis() bool { return cl.use > 0 || cl.add > 0 || cl.viaAdd }

// take folds one symbol's hit into the claim.
func (cl *reuseClaim) take(id graph.ID, h reuseHit, reused bool) {
	cl.use, cl.add, cl.mass = max(cl.use, h.use), max(cl.add, h.add), max(cl.mass, h.mass*h.precision)
	cl.reused = cl.reused || reused
	if callable(id) {
		cl.called = true
		cl.nodes = append([]graph.ID{id}, cl.nodes...)
	} else {
		cl.nodes = append(cl.nodes, id)
	}
}

// before is the reuse rank: use, via the addition's code, add, inside,
// reused, mass, path.
func (cl reuseClaim) before(o reuseClaim) bool {
	switch {
	case cl.use != o.use:
		return cl.use > o.use
	case cl.viaAdd != o.viaAdd:
		return cl.viaAdd
	case cl.add != o.add:
		return cl.add > o.add
	case cl.in != o.in:
		return cl.in
	case cl.reused != o.reused:
		return cl.reused
	case cl.mass != o.mass:
		return cl.mass > o.mass
	}
	return cl.file < o.file
}

// reuseHit is one node's unspent mass: over every token, over the use
// phrases' and over the addition phrases', with the tokens it named,
// whether its name holds a token of those phrases, and the share of its
// name's tokens the request's words hold.
type reuseHit struct {
	mass, use, add, precision float64
	name                      bool
	named                     map[string]bool
}

// reuseHits is the node's idf mass on the unspent request tokens its
// name (at WeightName) and doc (at WeightSummary, where the name lacks
// the token and the token names code somewhere) hold, split by the
// phrase class the token is in; its precision is the share of the name's
// tokens the request holds, spent or not.
func reuseHits(f Fields, spent, uses, adds, named map[string]bool, weights map[string]float64) reuseHit {
	h := reuseHit{named: map[string]bool{}}
	count := func(tokens []string, w float64) {
		for _, t := range tokens {
			if _, asked := weights[t]; !asked || spent[t] || h.named[t] || (w == WeightSummary && !named[t]) {
				continue
			}
			h.named[t] = true
			h.mass += weights[t] * w
			if uses[t] {
				h.use += weights[t] * w
			}
			if adds[t] {
				h.add += weights[t] * w
			}
			h.name = h.name || (w == WeightName && (uses[t] || adds[t]))
		}
	}
	count(f.Name, WeightName)
	count(f.Summary, WeightSummary)
	held := 0
	for _, t := range uniq(f.Name) {
		if _, asked := weights[t]; asked {
			held++
		}
	}
	if held > 0 {
		h.precision = float64(held) / float64(len(uniq(f.Name)))
	}
	return h
}

// claims is whether the hit holds some addition phrase's head (its
// heaviest token that names code: the word that makes the phrase a
// name) and at least ReuseFloor of the phrase's mass over those tokens.
func (h reuseHit) claims(c corpus, phrases []Phrase, weights map[string]float64) bool {
	for _, p := range phrases {
		var of, got, top float64
		head := ""
		for _, t := range keys(c.named(p.Tokens)) {
			of += weights[t]
			if h.named[t] {
				got += weights[t]
			}
			if weights[t] > top || (weights[t] == top && t < head) {
				top, head = weights[t], t
			}
		}
		if of > 0 && h.named[head] && got/of >= ReuseFloor {
			return true
		}
	}
	return false
}

// phraseTokens is the set of the phrases' tokens.
func phraseTokens(ps []Phrase) map[string]bool {
	out := map[string]bool{}
	for _, p := range ps {
		for _, t := range p.Tokens {
			out[t] = true
		}
	}
	return out
}

// usedByUnit is whether a symbol of the unit at path calls, may call or
// references the symbol.
func usedByUnit(g Reader, c corpus, id graph.ID, path string) bool {
	for _, e := range g.In(id, c.rev, graph.Calls, graph.MayCall, graph.References) {
		if i, ok := c.index[e.Src]; ok && c.units[i] == path {
			return true
		}
	}
	return false
}

// callable is whether a symbol is one to call or instantiate: a function,
// a method or a type, by the kind field of its id; a constant, a variable
// or a file is not an abstraction.
func callable(id graph.ID) bool {
	parts := strings.Split(string(id), ":")
	if parts[0] != "sym" || len(parts) < 4 {
		return false
	}
	switch parts[3] {
	case "func", "method", "type", "class":
		return true
	}
	return false
}

// calledOutside is whether code outside the unit at path calls, may call
// or references the symbol.
func calledOutside(g Reader, c corpus, id graph.ID, path string) bool {
	for _, e := range g.In(id, c.rev, graph.Calls, graph.MayCall, graph.References) {
		if i, ok := c.index[e.Src]; ok && c.units[i] != path {
			return true
		}
	}
	return false
}
