package engine

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// MaxLines is the delivery budget: the lines the brief holds under its
// header, one slot each (render.GraphLineSlots); the bytes are the briefing
// cap's, which the ladder in Build keeps by dropping lines.
const MaxLines = 5

// Line is one delivered line: the file, table, lesson or decision it
// stands for, the candidates it carries, its score (the best member's), and
// for a lesson or decision the file line it anchors to, the best-scored
// line it applies to. Slot is its facts as the slot holds them and Text the
// sentence the slot rendered, both set by Build.
type Line struct {
	Key     graph.ID
	Score   float64
	Members []graph.ID
	Anchor  graph.ID
	Slot    slot.GraphLine
	Text    string
}

// Decision is what Select did with one candidate.
type Decision struct {
	ID   graph.ID
	Line int
	Why  string
}

// Select groups candidates by the file or table that would carry them,
// orders the groups by their best score, and keeps MaxLines. A lesson or
// decision that applies in the neighborhood is a line of its own, scored
// as the best line it applies to, so its words arrive whole beside that
// line; a table line that only repeats what other lines carry is dropped;
// units and modules take none.
func Select(g Reader, rev graph.Rev, cands []Candidate) ([]Line, []Decision) {
	groups := map[graph.ID]*Line{}
	var order []graph.ID
	var decisions []Decision
	member := map[graph.ID]graph.ID{}
	for _, c := range cands {
		key, ok := lineKey(g, rev, c)
		if !ok {
			continue
		}
		l := groups[key]
		if l == nil {
			l = &Line{Key: key}
			groups[key] = l
			order = append(order, key)
		}
		l.Members = append(l.Members, c.ID)
		if c.Score > l.Score {
			l.Score = c.Score
		}
		member[c.ID] = key
	}
	for _, c := range cands {
		if !IsKnowledge(c.Kind) {
			continue
		}
		l := &Line{Key: c.ID, Members: []graph.ID{c.ID}, Score: c.Score}
		anchorScore := 0.0
		for _, m := range cands {
			key, ok := member[m.ID]
			if !ok || !slices.Contains(m.Knowledge, c.ID) {
				continue
			}
			l.Score = max(l.Score, groups[key].Score)
			if anchors(key, groups[key].Score, l.Anchor, anchorScore) {
				l.Anchor, anchorScore = key, groups[key].Score
			}
		}
		groups[c.ID] = l
		order = append(order, c.ID)
		member[c.ID] = c.ID
	}
	sort.SliceStable(order, func(i, j int) bool {
		if groups[order[i]].Score != groups[order[j]].Score {
			return groups[order[i]].Score > groups[order[j]].Score
		}
		return order[i] < order[j]
	})
	order = dropRepeatedHubs(g, rev, groups, order)
	index := map[graph.ID]int{}
	var lines []Line
	for i, key := range order {
		if i < MaxLines {
			index[key] = i + 1
			lines = append(lines, *groups[key])
		}
	}
	for _, c := range cands {
		key, ok := member[c.ID]
		if !ok {
			decisions = append(decisions, Decision{ID: c.ID, Why: "skipped: " + c.Kind})
			continue
		}
		if n, kept := index[key]; kept {
			decisions = append(decisions, Decision{ID: c.ID, Line: n, Why: "selected"})
		} else {
			decisions = append(decisions, Decision{ID: c.ID, Why: "cut: budget (" + strings.TrimSpace(fmtRank(groups[key], order)) + ")"})
		}
	}
	return lines, decisions
}

// anchors is whether key, at score, is a better anchor than cur at
// curScore: a file line over a table line, then the higher score, then the
// lower id.
func anchors(key graph.ID, score float64, cur graph.ID, curScore float64) bool {
	file, curFile := strings.HasPrefix(string(key), "file:"), strings.HasPrefix(string(cur), "file:")
	switch {
	case cur == "":
		return true
	case file != curFile:
		return file
	case score != curScore:
		return score > curScore
	}
	return key < cur
}

// dropRepeatedHubs gives a table line's slot to the best writer or reader
// line not yet within the budget, when there is one: the table's name is
// on every such line already, and a line names a path the agent can open.
func dropRepeatedHubs(g Reader, rev graph.Rev, groups map[graph.ID]*Line, order []graph.ID) []graph.ID {
	out := slices.Clone(order)
	for i := 0; i < len(out) && i < MaxLines; i++ {
		key := out[i]
		if !strings.HasPrefix(string(key), "tbl:") {
			continue
		}
		pos := -1
		for _, e := range g.In(key, rev, graph.Writes, graph.Reads) {
			if p := position(g, rev, e.Src, out); p >= MaxLines && (pos < 0 || p < pos) {
				pos = p
			}
		}
		if pos < 0 {
			continue
		}
		promoted := out[pos]
		out = slices.Delete(out, pos, pos+1)
		out[i] = promoted
		out = append(out, key)
	}
	return out
}

// position is the index of id's line in order, or -1.
func position(g Reader, rev graph.Rev, id graph.ID, order []graph.ID) int {
	key, ok := lineKey(g, rev, Candidate{ID: id, Kind: "sym"})
	if !ok {
		key = id
	}
	return slices.Index(order, key)
}

// lineKey is the line a candidate belongs to: a symbol's declaring file,
// a file or table itself.
func lineKey(g Reader, rev graph.Rev, c Candidate) (graph.ID, bool) {
	switch c.Kind {
	case "file", "table":
		return c.ID, true
	case "sym":
		for _, e := range g.In(c.ID, rev, graph.Declares) {
			return e.Src, true
		}
	}
	return "", false
}

func fmtRank(l *Line, order []graph.ID) string {
	for i, k := range order {
		if k == l.Key {
			return "group rank " + strconv.Itoa(i+1) + " of " + strconv.Itoa(len(order))
		}
	}
	return ""
}
