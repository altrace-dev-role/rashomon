package engine

import (
	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// roleLines is the lines the roles become, in the order the brief
// delivers them: the unit, the target file, the first reuse, consumer and
// knowledge, then the rest of each. A knowledge line is anchored to the
// file it was found through. A member the candidates do not hold is
// entered in byID with the knowledge that applies to it, so the facts
// stage reads every line the same way.
func roleLines(g Reader, rev graph.Rev, r Roles, byID map[graph.ID]Candidate) []Line {
	if !r.Found() {
		return nil
	}
	lines := []Line{{Key: r.Unit, Role: RoleUnit, Members: r.Targets, Consumers: r.UnitConsumers}}
	if r.File != "" {
		lines = append(lines, Line{Key: r.File, Role: RoleTarget, Members: append([]graph.ID{r.File}, r.Targets...)})
	}
	file := func(role string, rf RoleFile) Line {
		return Line{Key: rf.File, Role: role, Members: append([]graph.ID{rf.File}, rf.Members...)}
	}
	know := func(id graph.ID) Line {
		anchor := r.KnowledgeVia[id]
		if anchor == "" {
			anchor = r.File
		}
		return Line{Key: id, Role: RoleKnowledge, Members: []graph.ID{id}, Anchor: anchor}
	}
	for i := 0; i < len(r.Reuse) || i < len(r.Consumers) || i < len(r.Knowledge); i++ {
		if i < len(r.Reuse) {
			lines = append(lines, file(RoleReuse, r.Reuse[i]))
		}
		if i < len(r.Consumers) {
			lines = append(lines, file(RoleConsumer, r.Consumers[i]))
		}
		if i < len(r.Knowledge) {
			lines = append(lines, know(r.Knowledge[i]))
		}
	}
	for _, l := range lines {
		for _, m := range append(l.Members, l.Key) {
			if _, ok := byID[m]; ok {
				continue
			}
			if n, ok := g.Node(m, rev); ok {
				byID[m] = Candidate{ID: m, Kind: n.Kind, Knowledge: knowledgeOf(g, m, rev)}
			}
		}
	}
	return lines
}

// merge puts the role lines first and the related lines Select kept after
// them, a related line a role line already stands for dropped, and cuts
// at MaxLines. Select's decisions are renumbered to the merged order; a
// related line the role lines displaced is cut with a decision saying so,
// and each role line gets one naming its role.
func merge(g Reader, rev graph.Rev, r Roles, related []Line, decisions []Decision, byID map[graph.ID]Candidate) ([]Line, []Decision) {
	lines := roleLines(g, rev, r, byID)
	at := map[graph.ID]int{}
	var out []Decision
	for i, l := range lines {
		at[l.Key] = i + 1
		d := Decision{ID: l.Key, Line: i + 1, Why: "role: " + l.Role}
		if i >= MaxLines {
			d = Decision{ID: l.Key, Why: "cut: budget (role " + l.Role + ")"}
		}
		out = append(out, d)
	}
	for _, l := range related {
		if _, dup := at[l.Key]; dup {
			continue
		}
		lines = append(lines, l)
		at[l.Key] = len(lines)
	}
	for _, d := range decisions {
		if d.Line > 0 {
			if n := at[related[d.Line-1].Key]; n <= MaxLines {
				d.Line = n
			} else {
				d = Decision{ID: d.ID, Why: "cut: budget (role lines)"}
			}
		}
		out = append(out, d)
	}
	if len(lines) > MaxLines {
		lines = lines[:MaxLines]
	}
	return lines, out
}
