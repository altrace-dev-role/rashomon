package engine

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// lineFact is the line's facts as the slot holds them, read from the graph
// (GRAPH-SLOT1): a file's declarations, constants, calls, tables and
// imports with the records that apply; a table's writers and readers; a
// lesson's or decision's id, the path it anchors to, its clause and its
// source. rel maps a graph path to the brief's form.
func lineFact(g Reader, rev graph.Rev, l Line, byID map[graph.ID]Candidate, rel func(string) string) slot.GraphLine {
	if n, ok := g.Node(l.Key, rev); ok && IsKnowledge(n.Kind) {
		return slot.NewKnowledgeLine(knowledgeFact(g, rev, n, l.Anchor, rel))
	}
	if strings.HasPrefix(string(l.Key), "unit:") {
		return slot.NewUnitLine(unitFact(g, rev, l, rel))
	}
	if strings.HasPrefix(string(l.Key), "tbl:") {
		return slot.NewTableLine(tableFact(g, rev, l, byID))
	}
	return slot.NewCodeLine(codeFact(g, rev, l, byID, rel))
}

// unitFact is a unit line's facts: its path, the target symbols it
// declares (the line's members), the files outside it that consume it,
// and the knowledge that applies to the unit itself.
func unitFact(g Reader, rev graph.Rev, l Line, rel func(string) string) slot.UnitFact {
	f := slot.UnitFact{Path: slot.RelPath(rel(unitPath(l.Key)))}
	for _, m := range l.Members {
		if strings.HasPrefix(string(m), "sym:") {
			f.Names = appendIdent(f.Names, short(m))
		}
	}
	for _, c := range l.Consumers {
		f.Consumers = append(f.Consumers, slot.RelPath(rel(strings.TrimPrefix(string(c), "file:"))))
	}
	f.Lessons = appendKnowledge(f.Lessons, knowledgeOf(g, l.Key, rev))
	return f
}

// codeFact is a file line's facts: the members declared in it, their
// relations in the order calls, writes, reads, imports, the knowledge
// that applies to any member, and the role the line holds.
func codeFact(g Reader, rev graph.Rev, l Line, byID map[graph.ID]Candidate, rel func(string) string) slot.CodeFact {
	f := slot.CodeFact{Path: slot.RelPath(rel(strings.TrimPrefix(string(l.Key), "file:")))}
	switch l.Role {
	case RoleReuse:
		f.Role = slot.RoleReuse
	case RoleConsumer:
		f.Role = slot.RoleConsumer
	}
	for _, m := range l.Members {
		c := byID[m]
		n, ok := g.Node(m, rev)
		if !ok {
			continue
		}
		if c.Kind == "sym" {
			if v, err := strconv.Atoi(n.Attrs["value"]); n.Attrs["symkind"] == "const" && err == nil {
				f.Consts = append(f.Consts, slot.ConstFact{Name: slot.Ident(short(m)), Value: v})
			} else {
				f.Names = appendIdent(f.Names, short(m))
			}
		}
		for _, e := range g.Out(m, rev, graph.Calls, graph.Reads, graph.Writes, graph.References) {
			switch e.Pred {
			case graph.Calls:
				f.Calls = appendIdent(f.Calls, short(e.Dst))
			case graph.Writes:
				f.Writes = appendTable(f.Writes, e)
			case graph.Reads:
				f.Reads = appendTable(f.Reads, e)
			case graph.References:
				f.Imports = appendIdent(f.Imports, module(e.Dst))
			}
		}
		f.Lessons = appendKnowledge(f.Lessons, c.Knowledge)
	}
	return f
}

// tableFact is a table line's facts: who writes it, who reads it, and the
// knowledge that applies to it.
func tableFact(g Reader, rev graph.Rev, l Line, byID map[graph.ID]Candidate) slot.TableFact {
	f := slot.TableFact{Table: slot.Ident(strings.TrimPrefix(string(l.Key), "tbl:"))}
	for _, e := range g.In(l.Key, rev, graph.Writes, graph.Reads) {
		if e.Pred == graph.Writes {
			f.Writers = appendIdent(f.Writers, qualified(e.Src))
		} else {
			f.Readers = appendIdent(f.Readers, qualified(e.Src))
		}
	}
	for _, m := range l.Members {
		f.Lessons = appendKnowledge(f.Lessons, byID[m].Knowledge)
	}
	return f
}

// knowledgeFact is a lesson's or decision's own line: its id, its path,
// the one clause the record declares (attr clause) and its source
// document. The path is the line's anchor (the file the line was found
// through: a role file, or the best-scored file line reached through a
// table or a symbol) when the record applies to it or to a symbol it
// declares, else the first file among its applies_to targets, else the
// anchor, else none.
func knowledgeFact(g Reader, rev graph.Rev, n graph.Node, anchor graph.ID, rel func(string) string) slot.KnowledgeFact {
	f := slot.KnowledgeFact{Kind: n.Kind, ID: slot.KnowledgeID(n.ID), Clause: slot.Approved(n.Attrs["clause"]), Source: slot.Path(n.Attrs["source"])}
	target := anchor
	for _, e := range g.Out(n.ID, rev, graph.AppliesTo) {
		if anchor != "" && declaring(g, rev, e.Dst) == anchor {
			target = anchor
			break
		}
		if strings.HasPrefix(string(e.Dst), "file:") && (target == anchor || e.Dst < target) {
			target = e.Dst
		}
	}
	if p, ok := strings.CutPrefix(string(target), "file:"); ok {
		f.Path = slot.RelPath(rel(p))
	}
	return f
}

// appendTable adds the table a reads or writes edge names, with its
// columns, once.
func appendTable(refs []slot.TableRef, e graph.Edge) []slot.TableRef {
	name := slot.Ident(strings.TrimPrefix(string(e.Dst), "tbl:"))
	for _, r := range refs {
		if r.Table == name {
			return refs
		}
	}
	ref := slot.TableRef{Table: name}
	if cols := e.Attrs["cols"]; cols != "" {
		for _, c := range strings.Split(cols, ",") {
			ref.Cols = append(ref.Cols, slot.Ident(c))
		}
	}
	return append(refs, ref)
}

func appendIdent(ids []slot.Ident, s string) []slot.Ident {
	id := slot.Ident(s)
	for _, x := range ids {
		if x == id {
			return ids
		}
	}
	return append(ids, id)
}

func appendKnowledge(ids []slot.KnowledgeID, add []graph.ID) []slot.KnowledgeID {
	for _, k := range add {
		id := slot.KnowledgeID(k)
		dup := false
		for _, x := range ids {
			dup = dup || x == id
		}
		if !dup {
			ids = append(ids, id)
		}
	}
	return ids
}

// qualified is a symbol's dotted name, a file's path, a table's name.
func qualified(id graph.ID) string {
	s := string(id)
	switch {
	case strings.HasPrefix(s, "sym:"):
		parts := strings.Split(s, ":")
		return parts[len(parts)-1]
	case strings.HasPrefix(s, "file:"):
		return strings.TrimPrefix(s, "file:")
	case strings.HasPrefix(s, "tbl:"):
		return strings.TrimPrefix(s, "tbl:")
	}
	return s
}

// short is a symbol's last name segment.
func short(id graph.ID) string {
	q := qualified(id)
	if i := strings.LastIndex(q, "."); i >= 0 && strings.HasPrefix(string(id), "sym:") {
		return q[i+1:]
	}
	return q
}

// module is the dotted module of a symbol.
func module(id graph.ID) string {
	q := qualified(id)
	if i := strings.LastIndex(q, "."); i >= 0 {
		return q[:i]
	}
	return q
}
