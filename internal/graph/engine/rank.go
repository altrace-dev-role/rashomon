package engine

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// KnowledgeBonus is what a candidate gains when a lesson or decision
// applies to it, directly or through one of its own edges.
const KnowledgeBonus = 0.5

// Candidate is one neighborhood node with its ranking terms.
type Candidate struct {
	ID       graph.ID
	Kind     string
	Distance int
	Tier     graph.Tier
	Conf     float64
	// Base is w(tier)*conf/(1+distance).
	Base float64
	// Lex is the scope score when the node seeded, else 0.
	Lex float64
	// Knowledge is the lessons and decisions that apply to the node.
	Knowledge []graph.ID
	// Reach is whether the node lies outside the hook's working directory
	// (Outside), the code the agent cannot read on demand.
	Reach bool
	Score float64
	// Via is the predicate and nearer node this one was reached through.
	Via string
}

// TierWeight is the rank weight of a record's tier.
func TierWeight(t graph.Tier) float64 {
	switch t {
	case graph.Proven, graph.Declared:
		return 1.0
	case graph.Algorithmic:
		return 0.9
	case graph.Observed:
		return 0.8
	case graph.Inferred:
		return 0.5
	}
	return 0.5
}

// IsKnowledge is whether a node is a lesson or a decision.
func IsKnowledge(kind string) bool {
	switch kind {
	case "lesson", "decision", "incident", "convention", "exception", "flow", "objective":
		return true
	}
	return false
}

// Rank scores the neighborhood: score = w(tier)*conf/(1+distance) +
// KnowledgeBonus when knowledge applies + ReachBonus when outside says the
// agent cannot read the node on demand; order by score, then lexical
// score, then id. Knowledge nodes rank with the rest but attach to lines
// rather than taking them.
func Rank(g Reader, nb Neighborhood, seeds []Seed, outside func(graph.ID) bool) []Candidate {
	lex := map[graph.ID]float64{}
	for _, s := range seeds {
		lex[s.ID] = s.Score
	}
	var out []Candidate
	for id, d := range nb.Distance {
		n, ok := g.Node(id, nb.Rev)
		if !ok || n.Kind == "unit" || n.Kind == "mod" {
			continue
		}
		c := Candidate{ID: id, Kind: n.Kind, Distance: d, Tier: n.Tier, Conf: n.Conf, Lex: lex[id]}
		c.Base = TierWeight(n.Tier) * n.Conf / float64(1+d)
		c.Knowledge = knowledgeOf(g, id, nb.Rev)
		c.Score = c.Base
		if len(c.Knowledge) > 0 {
			c.Score += KnowledgeBonus
		}
		if c.Reach = outside(id); c.Reach {
			c.Score += ReachBonus
		}
		c.Via = via(g, id, d, nb)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Lex != b.Lex {
			return a.Lex > b.Lex
		}
		return a.ID < b.ID
	})
	return out
}

// knowledgeOf is the knowledge applying to id: directly, or to a table id
// reads or writes (a lesson on a table concerns its writers and readers;
// a lesson on a function does not pass to its callers).
func knowledgeOf(g Reader, id graph.ID, rev graph.Rev) []graph.ID {
	seen := map[graph.ID]bool{}
	var out []graph.ID
	add := func(target graph.ID) {
		for _, e := range g.In(target, rev, graph.AppliesTo) {
			if !seen[e.Src] {
				seen[e.Src] = true
				out = append(out, e.Src)
			}
		}
	}
	add(id)
	for _, e := range g.Out(id, rev, graph.Reads, graph.Writes) {
		add(e.Dst)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// via names the first edge from a node one step nearer: "<pred> <id>" for
// an in-edge, "<pred> -> <id>" for an out-edge; "seed" at distance 0.
func via(g Reader, id graph.ID, d int, nb Neighborhood) string {
	if d == 0 {
		return "seed"
	}
	preds := traversed()
	for _, e := range g.Out(id, nb.Rev, preds...) {
		if nb.Distance[e.Dst] == d-1 {
			return string(e.Pred) + " -> " + string(e.Dst)
		}
	}
	for _, e := range g.In(id, nb.Rev, preds...) {
		if nb.Distance[e.Src] == d-1 {
			return string(e.Src) + " " + string(e.Pred)
		}
	}
	return "?"
}
