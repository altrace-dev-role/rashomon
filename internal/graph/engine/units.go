package engine

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Unit resolution (W-M1.12): the unit that owns the target phrase is the
// one whose path and names cover the phrase's tokens best. The phrase is a
// qualified name: a unit whose path covers part of it and one of whose
// names covers the rest owns it over a unit where one name covers all of
// it (the wrapper named after the thing it wraps), which owns it over a
// unit whose names together cover it, which owns it over a part. A unit
// whose path alone covers the phrase owns it outright. A claim is made by
// callables and files: a constant or a variable whose name covers the
// phrase is a label for the thing, not the thing, so it adds coverage but
// completes no claim. Ties on the same tokens are settled by the claim's
// kind (a callable or file over a label), then by the precision of the
// covering name (the file or function named as the phrase over the one
// named after it), then by how much of the rest of the prompt the unit's
// names cover; what still ties, or ties on different tokens, is settled
// by the call direction between the tied units (the callee owns) or
// refused. A claim under half the phrase's weight is refused. Nothing is
// faked.

// UnitFloor is the least share of the phrase's weight a claim needs.
const UnitFloor = 0.5

// The classes of a claim, strongest first.
const (
	ClassUnit      = "unit"
	ClassQualified = "qualified"
	ClassSingle    = "single"
	ClassFull      = "full"
	ClassPartial   = "partial"
)

// UnitPick is one unit's claim on the phrase: its class, the weight of the
// phrase it covers out of the phrase's weight, the tokens it covers,
// whether a callable or file covers one, the precision of its best
// covering name (covered tokens over the name's tokens), the weight of
// the rest of the prompt its names hold (every name: a constant named
// for the prompt's words says what the unit is about as surely as a
// function, W-M1.32), the nodes whose
// names cover a token (best first), and why the claim was refused when it
// was.
type UnitPick struct {
	ID        graph.ID
	Path      string
	Class     string
	Covered   float64
	Of        float64
	Tokens    []string
	Callable  bool
	Precision float64
	Mass      float64
	Nodes     []graph.ID
	Refused   string
	// Tied is how many picks from the front tie when the claim is refused
	// as ambiguous, so a later signal can settle among them.
	Tied int
}

type claimNode struct {
	id       graph.ID
	name     float64
	all      float64
	complete bool
	callable bool
}

type unitClaim struct {
	path      string
	size      int
	need      map[string]bool
	covered   map[string]bool
	counted   map[string]bool
	complete  bool
	single    bool
	callable  bool
	precision float64
	mass      float64
	nodes     []claimNode
}

func classRank(c string) int {
	switch c {
	case ClassUnit:
		return 0
	case ClassQualified:
		return 1
	case ClassSingle:
		return 2
	case ClassFull:
		return 3
	}
	return 4
}

// Units is every unit's claim on the target tokens, best first, the
// first settled: tokens no name or path holds, and tokens the scorer
// gave no weight (a word in a path no unit owns, never scored), are left
// out of the phrase: a word that weighs nothing names nothing. rest is
// the prompt's other tokens, weighed for the tie-break.
func Units(g Reader, c corpus, target, rest []string, weights map[string]float64) []UnitPick {
	var T []string
	of, unknown, seen := 0.0, 0, 0.0
	known := func(t string) bool { return c.nameDF[t] > 0 && weights[t] > 0 }
	for _, t := range target {
		switch {
		case known(t):
			T = append(T, t)
			of += weights[t]
		case weights[t] > 0:
			seen += weights[t]
		default:
			unknown++
		}
	}
	if len(T) == 0 {
		return nil
	}
	// A word no name or path holds still weighs in what the phrase is of:
	// at the scorer's weight when the tree mentions it ("workflow" in a
	// doc sentence: a thing the tree has and no unit names), else at the
	// mean of the known words. A phrase mostly of such words names a
	// thing the index lacks, and the floor refuses it.
	of += float64(unknown)*of/float64(len(T)) + seen
	R := map[string]bool{}
	for _, t := range rest {
		R[t] = true
	}
	claims := map[string]*unitClaim{}
	for u := range c.unitID {
		claims[u] = newClaim(u, T)
	}
	for i, n := range c.nodes {
		cl := claims[c.units[i]]
		if cl == nil {
			continue
		}
		for _, t := range c.fields[i].Name {
			if R[t] && !cl.counted[t] {
				cl.counted[t] = true
				cl.mass += weights[t]
			}
		}
		cl.add(n.ID, c.fields[i], T, weights)
	}
	head := target[len(target)-1]
	var picks []UnitPick
	for _, cl := range claims {
		if len(cl.covered) == 0 {
			continue
		}
		picks = append(picks, cl.pick(c.unitID[cl.path], of, weights))
	}
	if known(head) && !anyCovers(picks, head) {
		return []UnitPick{{Class: ClassPartial, Of: of, Refused: "no unit names the head '" + head + "'"}}
	}
	sort.Slice(picks, func(i, j int) bool {
		a, b := picks[i], picks[j]
		if ra, rb := classRank(a.Class), classRank(b.Class); ra != rb {
			return ra < rb
		}
		if a.Covered != b.Covered {
			return a.Covered > b.Covered
		}
		return a.Path < b.Path
	})
	return settle(g, c, picks, len(T) == 1)
}

// anyCovers is whether any pick's tokens hold t.
func anyCovers(picks []UnitPick, t string) bool {
	for _, p := range picks {
		if hasToken(p.Tokens, t) {
			return true
		}
	}
	return false
}

func hasToken(toks []string, t string) bool {
	for _, x := range toks {
		if x == t {
			return true
		}
	}
	return false
}

// claimant is whether a node can make a claim: a callable symbol or a
// file; a constant or a variable is a label.
func claimant(id graph.ID) bool { return callable(id) || strings.HasPrefix(string(id), "file:") }

func newClaim(u string, T []string) *unitClaim {
	cl := &unitClaim{path: u, size: len(T), need: map[string]bool{}, covered: map[string]bool{}, counted: map[string]bool{}}
	own := map[string]bool{}
	for _, t := range Tokens(path.Base(u)) {
		own[t] = true
	}
	for _, t := range T {
		if own[t] {
			cl.covered[t] = true
		} else {
			cl.need[t] = true
		}
	}
	return cl
}

// add records a node's hits on the phrase: its name's, its path's, and
// whether its name completes what the unit path leaves (or, when the path
// leaves nothing, touches the phrase at all). Only a claimant completes.
func (cl *unitClaim) add(id graph.ID, f Fields, T []string, weights map[string]float64) {
	name, path := hits(f.Name, T), hits(f.Path, T)
	if len(name) == 0 && len(path) == 0 {
		return
	}
	n := claimNode{id: id, callable: claimant(id)}
	for _, t := range keys(name) {
		cl.covered[t] = true
		n.name += weights[t]
		n.all += weights[t]
	}
	for _, t := range keys(path) {
		cl.covered[t] = true
		if !name[t] {
			n.all += weights[t]
		}
	}
	if n.callable && len(name) > 0 {
		n.complete = len(cl.need) > 0 && covers(name, cl.need) || len(cl.need) == 0
		cl.complete = cl.complete || n.complete
		cl.single = cl.single || len(name) == len(T)
		cl.callable = true
		if p := float64(len(name)) / float64(len(uniq(f.Name))); p > cl.precision {
			cl.precision = p
		}
	}
	if len(name) > 0 {
		cl.nodes = append(cl.nodes, n)
	}
}

func (cl *unitClaim) pick(id graph.ID, of float64, weights map[string]float64) UnitPick {
	p := UnitPick{ID: id, Path: cl.path, Of: of, Callable: cl.callable, Precision: cl.precision, Mass: cl.mass, Tokens: keys(cl.covered)}
	for _, t := range p.Tokens {
		p.Covered += weights[t]
	}
	switch {
	case len(cl.need) == 0:
		p.Class = ClassUnit
	case cl.complete && len(cl.need) < cl.size:
		p.Class = ClassQualified
	case cl.complete || cl.single:
		p.Class = ClassSingle
	case len(cl.covered) == cl.size:
		p.Class = ClassFull
	default:
		p.Class = ClassPartial
	}
	sort.SliceStable(cl.nodes, func(i, j int) bool {
		a, b := cl.nodes[i], cl.nodes[j]
		if a.complete != b.complete {
			return a.complete
		}
		if a.callable != b.callable {
			return a.callable
		}
		if a.name != b.name {
			return a.name > b.name
		}
		if a.all != b.all {
			return a.all > b.all
		}
		return a.id < b.id
	})
	for _, n := range cl.nodes {
		p.Nodes = append(p.Nodes, n.id)
	}
	return p
}

func hits(field, T []string) map[string]bool {
	in := map[string]bool{}
	for _, t := range field {
		in[t] = true
	}
	out := map[string]bool{}
	for _, t := range T {
		if in[t] {
			out[t] = true
		}
	}
	return out
}

// keys is a set's members in sorted order: every sum over a set runs in
// one order, so equal sets give equal floats and ties stay ties.
func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func covers(have, need map[string]bool) bool {
	for t := range need {
		if !have[t] {
			return false
		}
	}
	return true
}

// settle refuses the first pick when it is under the floor, and when
// others tie with it on class and coverage settles the tie: on the same
// tokens, by the claim's kind, the precision of its best name and the
// rest of the prompt it names; then by the call direction, the one unit
// the others call and that calls none of them; else refused as ambiguous.
func settle(g Reader, c corpus, picks []UnitPick, single bool) []UnitPick {
	if len(picks) == 0 {
		return picks
	}
	if picks[0].Covered < UnitFloor*picks[0].Of {
		picks[0].Refused = "below the floor"
		return picks
	}
	n := 1
	for n < len(picks) && picks[n].Class == picks[0].Class && picks[n].Covered == picks[0].Covered {
		n++
	}
	if n == 1 {
		return picks
	}
	before := func(a, b UnitPick) bool { return nameBefore(a, b, single) }
	if sameTokens(picks[:n]) {
		sort.SliceStable(picks[:n], func(i, j int) bool { return before(picks[i], picks[j]) })
		if before(picks[0], picks[1]) {
			return picks
		}
		for n = 1; n < len(picks) && !before(picks[0], picks[n]) && picks[n].Class == picks[0].Class && picks[n].Covered == picks[0].Covered; n++ {
		}
	}
	if sink := calledUnit(g, c, picks[:n]); sink >= 0 {
		picks[0], picks[sink] = picks[sink], picks[0]
		return picks
	}
	picks[0].Refused, picks[0].Tied = "ambiguous: "+strconv.Itoa(n)+" units tie", n
	if single {
		picks[0].Refused = "ambiguous: '" + strings.Join(picks[0].Tokens, " ") + "' names code in " + strconv.Itoa(n) + " units"
	}
	return picks
}

// nameBefore is whether a's claim beats b's on the same tokens: a
// callable or file over a label, then the more precise name, then the
// more of the rest of the prompt named. A one-word phrase takes the
// prompt's evidence first: which unit's name is the tidier says less
// about what one word means than the rest of the prompt does.
func nameBefore(a, b UnitPick, single bool) bool {
	if a.Callable != b.Callable {
		return a.Callable
	}
	if single && a.Mass != b.Mass {
		return a.Mass > b.Mass
	}
	if a.Precision != b.Precision {
		return a.Precision > b.Precision
	}
	return a.Mass > b.Mass
}

func sameTokens(picks []UnitPick) bool {
	for _, p := range picks[1:] {
		if strings.Join(p.Tokens, " ") != strings.Join(picks[0].Tokens, " ") {
			return false
		}
	}
	return true
}

// calledUnit is the index of the one tied unit that another tied unit
// calls, references or may call and that calls none of them, or -1.
func calledUnit(g Reader, c corpus, tied []UnitPick) int {
	members := map[string][]graph.ID{}
	for i, n := range c.nodes {
		members[c.units[i]] = append(members[c.units[i]], n.ID)
	}
	owner := map[graph.ID]int{}
	for i, p := range tied {
		targets := p.Nodes
		if len(targets) == 0 {
			targets = members[p.Path]
		}
		for _, id := range targets {
			owner[id] = i
		}
	}
	in, out := make([]int, len(tied)), make([]int, len(tied))
	for i, p := range tied {
		for _, src := range members[p.Path] {
			for _, e := range g.Out(src, c.rev, graph.Calls, graph.MayCall, graph.References) {
				if j, ok := owner[e.Dst]; ok && j != i {
					out[i]++
					in[j]++
				}
			}
		}
	}
	sink := -1
	for i := range tied {
		if in[i] > 0 && out[i] == 0 {
			if sink >= 0 {
				return -1
			}
			sink = i
		}
	}
	return sink
}
