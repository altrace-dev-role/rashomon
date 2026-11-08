package engine

import (
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
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

// The basis words of a unit claim, the slot's (graph_unit.go): Named for
// a claim by a path, a declared name or a knowledge id the request held,
// UnitName for the unit's own name in the request, Coverage for a claim
// the ladder settled by word coverage (full, partial, qualified, single).
const (
	BasisNamed    = slot.BasisNamed
	BasisUnitName = slot.BasisUnitName
	BasisCoverage = slot.BasisCoverage
)

// RoleFile is a file in a role with the symbols of it the role rests on;
// a reuse file carries the basis word its line states.
type RoleFile struct {
	File    graph.ID
	Members []graph.ID
	Basis   string
}

// Roles is what the stage found: the frame, the phrase it resolved (the
// whole prompt when the frame named no target), every unit's claim on it,
// and the roles around the settled unit; KnowledgeVia is the file each
// knowledge id was found through, the file its line names. Why names the
// claim's class or the refusal; Basis is the claim's basis word and
// RunnerUp the ladder's next unrefused pick on another unit, both on the
// unit line (S3).
type Roles struct {
	Frame         Frame
	Named         Named
	Target        []string
	Derived       map[string]string
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
	Basis         string
	RunnerUp      graph.ID
}

// basisOf is the basis word of a claim of class c.
func basisOf(c string) string {
	switch c {
	case ClassNamed:
		return BasisNamed
	case ClassUnit:
		return BasisUnitName
	}
	return BasisCoverage
}

// runnerUp is the ladder's second pick when it is unrefused and on
// another unit than the first, else none.
func runnerUp(picks []UnitPick) graph.ID {
	if len(picks) > 1 && picks[1].Refused == "" && picks[1].ID != "" && picks[1].ID != picks[0].ID {
		return picks[1].ID
	}
	return ""
}

// Found is whether a unit was settled on.
func (r Roles) Found() bool { return r.Unit != "" }

// FindRoles resolves the frame's targets in order to the first unit a
// claim settles on, then reads the other roles from the graph around it.
func FindRoles(g Reader, c corpus, rows []ScopeRow, weights map[string]float64, frame Frame, named Named, prompt []string, outside func(graph.ID) bool) Roles {
	r := Roles{Frame: frame, Named: named}
	r.resolve(g, c, prompt, weights)
	if len(r.Picks) == 0 || r.Picks[0].Refused != "" {
		r.Knowledge, r.KnowledgeVia = leadKnowledge(named.Knowledge, nil, nil, "")
		return r
	}
	pick := r.Picks[0]
	r.Unit, r.Path = pick.ID, pick.Path
	r.Basis, r.RunnerUp = basisOf(pick.Class), runnerUp(r.Picks)
	r.File, r.Targets = targetFile(g, c, pick)
	r.Consumers, r.UnitConsumers = consumers(g, c, r, rows, outside)
	r.Reuse, r.ReuseClaims = reuse(g, c, rows, r, weights)
	r.Knowledge, r.KnowledgeVia = roleKnowledge(g, c, r)
	r.Knowledge, r.KnowledgeVia = leadKnowledge(named.Knowledge, r.Knowledge, r.KnowledgeVia, r.File)
	return r
}

// resolve settles the unit: the named claim (R0) unless an anchor phrase
// resolves elsewhere; a named place the index lacks refuses; else the
// frame's targets in order, each by the unit it names or by the ladder,
// the whole prompt when it named none; else the evidence of a named
// record, which also outranks a partial claim on a phrase (W-M1.31, N7:
// "B374 again; fix the root cause" is B374's unit, not the unit that
// declares a Cause). A first phrase that failed keeps its reason, and an
// owner phrase that claims nothing ends the search: the owned part is
// in it.
func (r *Roles) resolve(g Reader, c corpus, prompt []string, weights map[string]float64) {
	named := r.Named
	if named.Unit != "" {
		r.Target, r.Picks, r.Why = Tokens(namedWord(named).Word), []UnitPick{namedPick(c, named)}, ClassNamed
		if a, ok := r.anchor(g, c, prompt, weights); ok && a.Path != named.Unit {
			r.Picks = append([]UnitPick{a}, r.Picks...)
			r.Why = "anchor " + a.Class + " over named '" + namedWord(named).Word + "' (unit " + named.Unit + ")"
		}
		return
	}
	for _, w := range named.Unknown {
		if w.Refuses {
			r.refuse("named '" + w.Word + "': " + w.Why)
			return
		}
	}
	phrases := r.Frame.Targets
	if len(phrases) == 0 {
		phrases, r.Fallback = []Phrase{{Tokens: prompt}}, true
	}
	var first []UnitPick
	firstWhy, over, pronoun := "", "", ""
	for _, p := range phrases {
		if p.Pronoun != "" && !c.holdsAny(p.Tokens) {
			pronoun = "'" + p.Pronoun + "' stands for a thing no clause names, and no name holds '" + strings.Join(p.Tokens, " ") + "'"
			continue
		}
		r.Target, r.Derived = c.derive(c.bare(p), r.Derived)
		weights = c.weighDerived(weights, r.Derived)
		r.Picks, r.Why = r.claim(g, c, p, prompt, weights)
		if len(r.Picks) > 0 && r.Picks[0].Refused == "" {
			if u := named.KnowledgeUnit; u != "" && u != "?" && u != r.Picks[0].Path && r.Picks[0].Class == ClassPartial {
				over = r.Picks[0].Path
				break
			}
			return
		}
		if first == nil && firstWhy == "" {
			first, firstWhy = r.Picks, r.Why
		}
		if p.Owner {
			break
		}
	}
	if u := named.KnowledgeUnit; u != "" && u != "?" {
		pick := UnitPick{ID: c.unitID[u], Path: u, Class: ClassNamed, Covered: 1, Of: 1, Callable: true, Precision: 1}
		for _, k := range named.Knowledge {
			pick.Nodes = append(pick.Nodes, c.evidenceIn(g, k, u)...)
		}
		r.Picks, r.Why = []UnitPick{pick}, "named '"+short(named.Knowledge[0])+"' applies to "+u
		if over != "" {
			r.Why += " over a partial claim on " + over
		}
		return
	}
	r.Picks, r.Why = first, firstWhy
	if len(r.Picks) == 0 && pronoun != "" {
		r.refuse(pronoun)
	}
	if len(named.Unknown) > 0 {
		r.refuse("named '" + named.Unknown[0].Word + "': " + named.Unknown[0].Why)
	}
}

// holdsAny is whether an index name holds one of the tokens.
func (c corpus) holdsAny(tokens []string) bool {
	for _, t := range tokens {
		if c.nameDF[t] > 0 {
			return true
		}
	}
	return false
}

// bare is the phrase's tokens less a trailing word in an adverb's form
// that no name holds ("resolve the prompt root lazily" is about the
// prompt root): the verb's manner weighs nothing in what the phrase is
// of. A name that holds the word keeps it as the head ("the message
// assembly" where assembly is a name).
func (c corpus) bare(p Phrase) []string {
	if p.Adverb == "" || c.nameDF[p.Adverb] > 0 || len(p.Tokens) < 2 {
		return p.Tokens
	}
	return p.Tokens[:len(p.Tokens)-1]
}

// refuse marks the resolution refused for why.
func (r *Roles) refuse(why string) {
	if len(r.Picks) == 0 {
		r.Picks = []UnitPick{{Class: ClassPartial}}
	}
	r.Picks[0].Refused, r.Why = why, "refused: "+why
}

// anchor is the claim the frame's anchor phrase settles on, when it has one.
func (r Roles) anchor(g Reader, c corpus, prompt []string, weights map[string]float64) (UnitPick, bool) {
	for _, p := range r.Frame.Targets {
		if !p.Anchor {
			continue
		}
		if picks := Units(g, c, p.Tokens, except(prompt, p.Tokens), weights); len(picks) > 0 && picks[0].Refused == "" {
			return picks[0], true
		}
	}
	return UnitPick{}, false
}

// namedPick is the claim a named word makes: whole, callable, on the
// word's node.
func namedPick(c corpus, n Named) UnitPick {
	p := UnitPick{ID: c.unitID[n.Unit], Path: n.Unit, Class: ClassNamed, Covered: 1, Of: 1, Callable: true, Precision: 1, Tokens: Tokens(namedWord(n).Word)}
	if n.Node != "" {
		p.Nodes = []graph.ID{n.Node}
	}
	return p
}

// namedWord is the word that resolved to the named unit.
func namedWord(n Named) NamedWord {
	for _, w := range n.Words {
		if w.Unit == n.Unit && w.Kind != NamedKnowledge {
			return w
		}
	}
	return NamedWord{}
}

// leadKnowledge puts the records the prompt named by id first (W-M1.15:
// knowledge as an entry point), found through the target file.
func leadKnowledge(lead, found []graph.ID, via map[graph.ID]graph.ID, file graph.ID) ([]graph.ID, map[graph.ID]graph.ID) {
	if len(lead) == 0 {
		return found, via
	}
	if via == nil {
		via = map[graph.ID]graph.ID{}
	}
	out := append([]graph.ID{}, lead...)
	seen := map[graph.ID]bool{}
	for _, k := range lead {
		seen[k] = true
		if _, ok := via[k]; !ok {
			via[k] = file
		}
	}
	for _, k := range found {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out, via
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

// roleKnowledge is the unit's own knowledge, the most specific first: on
// the target symbols and file, then on the unit's other files and the
// symbols they declare (each through its file), then on the unit node;
// within a step a requirement before a lesson before a decision, then by
// id. A record on a reuse or consumer file is that line's to show ("X
// applies" on the code line) and never a knowledge line (W-M1.23,
// AC-M1.5): a unit with no record of its own gets none, and a
// neighbour's record is not the unit's. On the W-M1.19 set 25 of 160
// knowledge lines stood on a unit with no record and 36 on a neighbour's.
// Each id maps to the file it was found through (the target file for the
// unit's own), so its line names that file and not another the record
// applies to.
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
	var first, own [][2]graph.ID
	for _, id := range append(append([]graph.ID{}, r.Targets...), r.File) {
		first = append(first, [2]graph.ID{id, r.File})
	}
	step(first...)
	for _, f := range unitFiles(g, c.rev, r.Unit) {
		if f == r.File {
			continue
		}
		own = append(own, [2]graph.ID{f, f})
		for _, e := range g.Out(f, c.rev, graph.Declares) {
			own = append(own, [2]graph.ID{e.Dst, f})
		}
	}
	step(own...)
	step([2]graph.ID{r.Unit, r.File})
	return out, via
}

// unitFiles is the files a unit contains, by id, so a record two of them
// carry is found through the same one on every run.
func unitFiles(g Reader, rev graph.Rev, unit graph.ID) []graph.ID {
	var files []graph.ID
	for _, e := range g.Out(unit, rev, graph.Contains) {
		files = append(files, e.Dst)
	}
	sort.Slice(files, func(i, j int) bool { return files[i] < files[j] })
	return files
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
