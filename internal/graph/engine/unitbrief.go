package engine

import (
	"slices"
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The first-write moment (W-M2.3, engine half). The prompt brief reaches
// depth 2 from the prompt's seeds; the first write names the unit, from
// which the consumers are In edges at depth 1. UnitBrief reads only the
// records and the prompt brief's lines: it takes no prompt word, so what
// it returns holds none (Brief.Prompt and Tokens stay empty).

// ruleKind is whether a knowledge kind binds the code it applies to, a
// rule or decision of the unit brief; every other knowledge kind is a
// lesson.
func ruleKind(kind string) bool { return kind == "decision" || kind == "requirement" }

// UnitBrief is the brief for the first write in unit at rev. Lines, in
// order: the consumers (files of other units with a Calls, MayCall or
// Imports edge into the unit's files or symbols, most edges first, then
// path), the rules and decisions whose applies_to reach the unit, its
// files or its symbols, the lessons by applies_to (each by id), then the
// prompt brief's reuse line for this unit when it had one. A line whose
// key the prompt brief already delivered is left out of the first three
// groups (Deltas), so the budget is spent on what is new; the reuse line
// is the prompt's own and rides last by design, a caller applying Deltas
// to the result drops it. MaxLines caps the whole. The rules come from
// the graph (knowledge nodes and their applies_to edges, the form
// knowledge.Rules compiles from), so no rules parameter is needed.
func UnitBrief(g Reader, rev graph.Rev, unit graph.ID, prompt Brief, opt Options) Brief {
	b := Brief{Rev: rev, Scorer: "unit"}
	targets := unitTargets(g, rev, unit)
	if len(targets) == 0 {
		return b
	}
	byID := map[graph.ID]Candidate{}
	var lines []Line
	for _, l := range consumerLines(g, rev, unit, targets) {
		lines = append(lines, l)
		for _, m := range l.Members {
			byID[m] = Candidate{ID: m, Kind: kindOfID(m)}
		}
	}
	rules, lessons := knowledgeLines(g, rev, unit, targets)
	lines = append(lines, rules...)
	lines = append(lines, lessons...)
	cur := Brief{Lines: lines}
	lines = Deltas(prompt, cur)
	for _, l := range prompt.Lines {
		if l.Role == RoleReuse && inUnitFile(g, rev, l.Key, unit) {
			lines = append(lines, Line{Key: l.Key, Role: RoleReuse, Basis: l.Basis, Band: l.Band, Members: l.Members, Score: l.Score})
			for _, m := range l.Members {
				byID[m] = Candidate{ID: m, Kind: kindOfID(m)}
			}
			break
		}
	}
	if len(lines) > MaxLines {
		lines = lines[:MaxLines]
	}
	rel := opt.rel()
	for i := range lines {
		lines[i].Slot = lineFact(g, rev, lines[i], byID, rel)
	}
	b.Lines = lines
	b.bind()
	b.deliver()
	return b
}

// Deltas is the lines of cur whose key is not a key of prev: the set
// difference on line keys, in cur's order, so a unit brief never repeats a
// line the prompt brief delivered.
func Deltas(prev, cur Brief) []Line {
	seen := map[graph.ID]bool{}
	for _, l := range prev.Lines {
		seen[l.Key] = true
	}
	var out []Line
	for _, l := range cur.Lines {
		if !seen[l.Key] {
			out = append(out, l)
		}
	}
	return out
}

// unitTargets is the unit itself, the files it contains and the symbols
// they declare, in id order within each.
func unitTargets(g Reader, rev graph.Rev, unit graph.ID) []graph.ID {
	if _, ok := g.Node(unit, rev); !ok {
		return nil
	}
	targets := []graph.ID{unit}
	for _, f := range unitFiles(g, rev, unit) {
		targets = append(targets, f)
		var syms []graph.ID
		for _, e := range g.Out(f, rev, graph.Declares) {
			syms = append(syms, e.Dst)
		}
		slices.Sort(syms)
		targets = append(targets, syms...)
	}
	return targets
}

// consumerLines is one consumer line per file outside the unit with an
// edge into it, most edges first; Members are the calling symbols.
func consumerLines(g Reader, rev graph.Rev, unit graph.ID, targets []graph.ID) []Line {
	count := map[graph.ID]int{}
	members := map[graph.ID][]graph.ID{}
	for _, t := range targets {
		for _, e := range g.In(t, rev, graph.Calls, graph.MayCall, graph.Imports) {
			file := declaring(g, rev, e.Src)
			if file == "" || inUnitFile(g, rev, file, unit) || inUnitFile(g, rev, e.Src, unit) {
				continue
			}
			count[file]++
			if e.Src != file && !slices.Contains(members[file], e.Src) {
				members[file] = append(members[file], e.Src)
			}
		}
	}
	files := make([]graph.ID, 0, len(count))
	for f := range count {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool {
		if count[files[i]] != count[files[j]] {
			return count[files[i]] > count[files[j]]
		}
		return files[i] < files[j]
	})
	lines := make([]Line, len(files))
	for i, f := range files {
		slices.Sort(members[f])
		lines[i] = Line{Key: f, Role: RoleConsumer, Score: float64(count[f]), Members: append([]graph.ID{f}, members[f]...)}
	}
	return lines
}

// knowledgeLines is the knowledge applying to any target, split into
// rules and decisions and lessons, each in id order, one line per record.
func knowledgeLines(g Reader, rev graph.Rev, unit graph.ID, targets []graph.ID) (rules, lessons []Line) {
	seen := map[graph.ID]bool{}
	var ids []graph.ID
	for _, t := range targets {
		for _, e := range g.In(t, rev, graph.AppliesTo) {
			if !seen[e.Src] {
				seen[e.Src] = true
				ids = append(ids, e.Src)
			}
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		n, ok := g.Node(id, rev)
		if !ok || !IsKnowledge(n.Kind) {
			continue
		}
		l := Line{Key: id, Role: RoleKnowledge, Members: []graph.ID{id}, Anchor: unit}
		if ruleKind(n.Kind) {
			rules = append(rules, l)
		} else {
			lessons = append(lessons, l)
		}
	}
	return rules, lessons
}

// inUnitFile is whether id is a file or symbol of unit.
func inUnitFile(g Reader, rev graph.Rev, id, unit graph.ID) bool {
	_, holder := unitOf(g, rev, graph.Node{ID: id})
	return holder == unit
}

// kindOfID is the node kind a line member's id prefix names, the two
// kinds codeFact reads.
func kindOfID(id graph.ID) string {
	if len(id) > 4 && id[:4] == "sym:" {
		return "sym"
	}
	return "file"
}
