package steer

import "github.com/altrace-dev-role/rashomon/internal/steer/render/slot"

// graphLineJSON is a graph line's form on the steer line (card GRAPH-SLOT1):
// flat arrays of raw values, keys in sorted order, so learn's delivered-set
// reader finds the knowledge ids with jsonscan and never decodes a line
// (RES-5). The record form (record scope v4) writes the path and the
// source as their keyed digests, each list as its count and no name; the
// columns a write names and a constant's value are rendered and not
// recorded: the brief's text is what the line holds them in. A unit line
// (W-M1.12) holds its path, names, consumer files and lessons; a code line
// its role word (slot.RoleReuse, slot.RoleConsumer) or none; a not-known
// line (W-M1.38) its kind alone.
type graphLineJSON struct {
	Calls        []string        `json:"calls"`
	Clause       string          `json:"clause"`
	Consumers    []string        `json:"consumers"`
	Counts       graphCountsJSON `json:"counts"`
	Digest       string          `json:"digest"`
	ID           string          `json:"id"`
	Imports      []string        `json:"imports"`
	Kind         string          `json:"kind"`
	Lessons      []string        `json:"lessons"`
	Names        []string        `json:"names"`
	Path         string          `json:"path"`
	Readers      []string        `json:"readers"`
	Reads        []string        `json:"reads"`
	Role         string          `json:"role"`
	Source       string          `json:"source"`
	SourceDigest string          `json:"source_digest"`
	Table        string          `json:"table"`
	Writers      []string        `json:"writers"`
	Writes       []string        `json:"writes"`
}

// graphCountsJSON is each list's count beyond the listed items.
type graphCountsJSON struct {
	Calls     int `json:"calls"`
	Consts    int `json:"consts"`
	Consumers int `json:"consumers"`
	Imports   int `json:"imports"`
	Lessons   int `json:"lessons"`
	Names     int `json:"names"`
	Readers   int `json:"readers"`
	Reads     int `json:"reads"`
	Writers   int `json:"writers"`
	Writes    int `json:"writes"`
}

// graphLineForm is one graph line as its steer line records it.
func graphLineForm(l slot.GraphLine) graphLineJSON {
	m := l.More()
	out := graphLineJSON{
		Calls: []string{}, Consumers: []string{}, Imports: []string{}, Lessons: []string{}, Names: []string{}, Readers: []string{},
		Reads: []string{}, Writers: []string{}, Writes: []string{},
		Counts: graphCountsJSON{Calls: m.Calls, Consts: m.Consts, Consumers: m.Consumers, Imports: m.Imports, Lessons: m.Lessons, Names: m.Names,
			Readers: m.Readers, Reads: m.Reads, Writers: m.Writers, Writes: m.Writes},
		Digest: l.Digest(), Kind: l.Kind(), SourceDigest: l.SourceDigest(),
	}
	switch {
	case l.Kind() == string(KindUnit):
		u, _ := l.Unit()
		out.Path, out.Names, out.Lessons = string(u.Path), identStrings(u.Names), idStrings(u.Lessons)
		for _, c := range u.Consumers {
			out.Consumers = append(out.Consumers, string(c))
		}
	case l.Kind() == "code":
		c, _ := l.Code()
		out.Path, out.Role = string(c.Path), c.Role
		out.Names = identStrings(c.Names)
		for _, k := range c.Consts {
			out.Names = append(out.Names, string(k.Name))
		}
		out.Calls, out.Imports, out.Lessons = identStrings(c.Calls), identStrings(c.Imports), idStrings(c.Lessons)
		for _, t := range c.Writes {
			out.Writes = append(out.Writes, string(t.Table))
		}
		for _, t := range c.Reads {
			out.Reads = append(out.Reads, string(t.Table))
		}
	case l.Kind() == "table":
		t, _ := l.Table()
		out.Table = string(t.Table)
		out.Writers, out.Readers, out.Lessons = identStrings(t.Writers), identStrings(t.Readers), idStrings(t.Lessons)
	case l.Kind() == slot.KindNotKnown:
	case l.Kind() != "":
		k, _ := l.Knowledge()
		out.ID, out.Path, out.Clause, out.Source = string(k.ID), string(k.Path), string(k.Clause), string(k.Source)
	}
	return out
}

func identStrings(items []slot.Ident) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = string(it)
	}
	return out
}

func idStrings(ids []slot.KnowledgeID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
