package engine

import (
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The roles stage (PLAN 5.2, W-M1.12): from the prompt's frame the brief
// names the unit that owns the task, the file the task enters it by, the
// abstractions to reuse, the consumers outside the unit, and the
// knowledge that applies, each read from the graph around the unit the
// phrase resolved to. A role the graph cannot prove is left out.

// MaxReuse and MaxConsumers cap the files a role names.
const (
	MaxReuse     = 3
	MaxConsumers = 3
)

// The role words a line carries, for the decisions and the record.
const (
	RoleUnit      = "unit"
	RoleTarget    = "target"
	RoleReuse     = "reuse"
	RoleConsumer  = "consumer"
	RoleKnowledge = "knowledge"
)

// RoleFile is a file in a role with the symbols of it the role rests on.
type RoleFile struct {
	File    graph.ID
	Members []graph.ID
}

// ReuseClaim is one file's claim on the reuse role, for Explain: its
// best symbol's mass on the use phrases, on the addition phrases and on
// every unspent token, whether the addition's existing code calls a hit
// symbol of it, whether it is inside the unit and whether code outside
// the unit already calls a hit symbol of it.
type ReuseClaim struct {
	File               graph.ID
	Use, Add, Mass     float64
	ViaAdd, In, Reused bool
	Nodes              int
}

// Roles is what the stage found: the frame, the phrase it resolved (the
// whole prompt when the frame named no target), every unit's claim on it,
// and the roles around the settled unit; KnowledgeVia is the file each
// knowledge id was found through, the file its line names. Why names the
// claim's class or the refusal.
type Roles struct {
	Frame         Frame
	Target        []string
	Fallback      bool
	Picks         []UnitPick
	Unit          graph.ID
	Path          string
	File          graph.ID
	Targets       []graph.ID
	Reuse         []RoleFile
	ReuseClaims   []ReuseClaim
	Consumers     []RoleFile
	UnitConsumers []graph.ID
	Knowledge     []graph.ID
	KnowledgeVia  map[graph.ID]graph.ID
	Why           string
}

// Found is whether a unit was settled on.
func (r Roles) Found() bool { return r.Unit != "" }

// FindRoles resolves the frame's targets in order to the first unit a
// claim settles on, then reads the other roles from the graph around it.
func FindRoles(g Reader, c corpus, rows []ScopeRow, weights map[string]float64, frame Frame, prompt []string, outside func(graph.ID) bool) Roles {
	r := Roles{Frame: frame}
	phrases := frame.Targets
	if len(phrases) == 0 {
		phrases, r.Fallback = []Phrase{{Tokens: prompt}}, true
	}
	for _, p := range phrases {
		r.Target, r.Picks = p.Tokens, Units(g, c, p.Tokens, except(prompt, p.Tokens), weights)
		switch {
		case len(r.Picks) == 0:
			r.Why = "no token of the target names code"
			continue
		case r.Picks[0].Refused != "":
			r.Why = "refused: " + r.Picks[0].Refused
			continue
		}
		r.Why = r.Picks[0].Class
		break
	}
	if len(r.Picks) == 0 || r.Picks[0].Refused != "" {
		return r
	}
	pick := r.Picks[0]
	r.Unit, r.Path = pick.ID, pick.Path
	r.File, r.Targets = targetFile(g, c, pick)
	r.Consumers, r.UnitConsumers = consumers(g, c, r, rows, outside)
	r.Reuse, r.ReuseClaims = reuse(g, c, rows, r, weights)
	r.Knowledge, r.KnowledgeVia = roleKnowledge(g, c, r)
	return r
}

// except is the tokens of all not in drop.
func except(all, drop []string) []string {
	skip := map[string]bool{}
	for _, t := range drop {
		skip[t] = true
	}
	var out []string
	for _, t := range all {
		if !skip[t] {
			out = append(out, t)
		}
	}
	return out
}

// targetFile is the file the request enters the unit by: the file
// declaring the pick's best node (that node when it is a file), with the
// covering symbols it declares.
func targetFile(g Reader, c corpus, pick UnitPick) (graph.ID, []graph.ID) {
	if len(pick.Nodes) == 0 {
		return "", nil
	}
	file := declaring(g, c.rev, pick.Nodes[0])
	if file == "" {
		return "", nil
	}
	var syms []graph.ID
	for _, id := range pick.Nodes {
		if strings.HasPrefix(string(id), "sym:") && declaring(g, c.rev, id) == file {
			syms = append(syms, id)
		}
	}
	return file, syms
}

// declaring is a file itself, a symbol's declaring file, "" for the rest.
func declaring(g Reader, rev graph.Rev, id graph.ID) graph.ID {
	if strings.HasPrefix(string(id), "file:") {
		return id
	}
	for _, e := range g.In(id, rev, graph.Declares) {
		return e.Src
	}
	return ""
}

// named is the target tokens a name or path holds.
func (c corpus) named(target []string) map[string]bool {
	out := map[string]bool{}
	for _, t := range target {
		if c.nameDF[t] > 0 {
			out[t] = true
		}
	}
	return out
}

// consumers is the files outside the unit that call, may call or
// reference the target symbols, reaching through a wrapper whose name
// covers the phrase to the wrapper's callers (the consumer role, at most
// MaxConsumers), and the files outside the unit that call or reference
// any symbol of it (the unit line's list: a possible call through an
// interface is no "consumed by"), the consumer role's first. When no file
// reaches a target symbol the first file calling the unit takes the
// role: the non-local consumer is the unit's caller, the target's when
// there is one. Within each, a file with knowledge on it comes first,
// then one outside the working directory (card GRAPH-REACH1: what the
// agent cannot open by name), then the one the prompt's words reach by
// its scope score, then more consuming symbols, then the path.
func consumers(g Reader, c corpus, r Roles, rows []ScopeRow, outside func(graph.ID) bool) ([]RoleFile, []graph.ID) {
	found := map[graph.ID]*RoleFile{}
	var order []graph.ID
	lex := map[graph.ID]float64{}
	for _, row := range rows {
		lex[row.ID] = row.Score
	}
	score := func(rf *RoleFile) float64 {
		best := lex[rf.File]
		for _, m := range rf.Members {
			best = max(best, lex[m])
		}
		return best
	}
	add := func(src graph.ID) *RoleFile {
		file := declaring(g, c.rev, src)
		i, ok := c.index[src]
		if file == "" || !ok || c.units[i] == r.Path {
			return nil
		}
		rf := found[file]
		if rf == nil {
			rf = &RoleFile{File: file}
			found[file] = rf
			order = append(order, file)
		}
		if src != file && !contains(rf.Members, src) {
			rf.Members = append(rf.Members, src)
		}
		return rf
	}
	T := c.named(r.Target)
	for _, t := range r.Targets {
		for _, e := range g.In(t, c.rev, graph.Calls, graph.MayCall, graph.References) {
			if add(e.Src) == nil || !wraps(c, e.Src, T) {
				continue
			}
			for _, e2 := range g.In(e.Src, c.rev, graph.Calls, graph.MayCall) {
				add(e2.Src)
			}
		}
	}
	rank := func(ids []graph.ID) {
		sort.SliceStable(ids, func(i, j int) bool {
			a, b := found[ids[i]], found[ids[j]]
			if ka, kb := hasKnowledge(g, c.rev, a), hasKnowledge(g, c.rev, b); ka != kb {
				return ka
			}
			if oa, ob := outside(a.File), outside(b.File); oa != ob {
				return oa
			}
			if sa, sb := score(a), score(b); sa != sb {
				return sa > sb
			}
			if len(a.Members) != len(b.Members) {
				return len(a.Members) > len(b.Members)
			}
			return a.File < b.File
		})
	}
	rank(order)
	role := make([]RoleFile, 0, len(order))
	for _, f := range order {
		role = append(role, *found[f])
	}
	if len(role) > MaxConsumers {
		role = role[:MaxConsumers]
	}
	var rest []graph.ID
	for i, n := range c.nodes {
		if c.units[i] != r.Path || !strings.HasPrefix(string(n.ID), "sym:") {
			continue
		}
		for _, e := range g.In(n.ID, c.rev, graph.Calls, graph.References) {
			if found[declaring(g, c.rev, e.Src)] == nil && add(e.Src) != nil {
				rest = append(rest, declaring(g, c.rev, e.Src))
			}
		}
	}
	rank(rest)
	if len(role) == 0 && len(rest) > 0 {
		role, rest = []RoleFile{*found[rest[0]]}, rest[1:]
	}
	unit := make([]graph.ID, 0, len(order))
	for _, rf := range role {
		unit = append(unit, rf.File)
	}
	return role, append(unit, rest...)
}

// wraps is whether a symbol's name covers every named target token: a
// wrapper named after the thing it wraps.
func wraps(c corpus, id graph.ID, T map[string]bool) bool {
	i, ok := c.index[id]
	if !ok || len(T) == 0 || !strings.HasPrefix(string(id), "sym:") {
		return false
	}
	name := map[string]bool{}
	for _, t := range c.fields[i].Name {
		name[t] = true
	}
	return covers(name, T)
}

func hasKnowledge(g Reader, rev graph.Rev, rf *RoleFile) bool {
	if len(knowledgeOf(g, rf.File, rev)) > 0 {
		return true
	}
	for _, m := range rf.Members {
		if len(knowledgeOf(g, m, rev)) > 0 {
			return true
		}
	}
	return false
}

func contains(ids []graph.ID, id graph.ID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// reuse is the abstraction to reuse: the files declaring callable
// symbols (functions, methods, types) whose names or docs hold the
// request's words beyond those that named the target (the unit's own
// path, the target tokens its symbols' names hold and the frame's verbs
// are spent; a doc's word counts when it names code somewhere), among
// the scope rows at or over SeedFloor, never the target file or a
// consumer. A file inside the unit, or one the unit already calls,
// counts on any hit. A file elsewhere counts when a symbol's name holds a
// word of what the request says to use (a use phrase: what the request
// presumes exists) or names at least ReuseFloor of an addition phrase's
// mass among the tokens that name code, as a unit claims a target: a
// word in common with what the request adds is a neighbour, a symbol
// that names most of it is the thing to take up, and a doc that mentions
// the word is neither. A constant or variable adds its hits to its
// file's claim but never carries it; a file's claim is its best symbol's.
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
		if cl.called && len(out) < MaxReuse {
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

// roleKnowledge is the knowledge applying to the roles, the most specific
// first: on the target symbols and file, then on the reuse and consumer
// files and their members, then on the unit; within a step a requirement
// before a lesson before a decision, then by id. Each id maps to the file
// it was found through (the target file for the unit's own), so its line
// names that file and not another the record applies to.
func roleKnowledge(g Reader, c corpus, r Roles) ([]graph.ID, map[graph.ID]graph.ID) {
	seen := map[graph.ID]bool{}
	via := map[graph.ID]graph.ID{}
	var out []graph.ID
	step := func(pairs ...[2]graph.ID) {
		var found []graph.ID
		for _, p := range pairs {
			for _, k := range knowledgeOf(g, p[0], c.rev) {
				if !seen[k] {
					seen[k] = true
					via[k] = p[1]
					found = append(found, k)
				}
			}
		}
		sort.SliceStable(found, func(i, j int) bool {
			if a, b := kindRank(g, c.rev, found[i]), kindRank(g, c.rev, found[j]); a != b {
				return a < b
			}
			return found[i] < found[j]
		})
		out = append(out, found...)
	}
	var first, hop [][2]graph.ID
	for _, id := range append(append([]graph.ID{}, r.Targets...), r.File) {
		first = append(first, [2]graph.ID{id, r.File})
	}
	step(first...)
	for _, rf := range append(append([]RoleFile{}, r.Reuse...), r.Consumers...) {
		for _, id := range append([]graph.ID{rf.File}, rf.Members...) {
			hop = append(hop, [2]graph.ID{id, rf.File})
		}
	}
	step(hop...)
	step([2]graph.ID{r.Unit, r.File})
	return out, via
}

// kindRank orders knowledge kinds for one step: requirement, lesson,
// decision, then the rest.
func kindRank(g Reader, rev graph.Rev, id graph.ID) int {
	n, _ := g.Node(id, rev)
	switch n.Kind {
	case "requirement":
		return 0
	case "lesson":
		return 1
	case "decision":
		return 2
	}
	return 3
}
