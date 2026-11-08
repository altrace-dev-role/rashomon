package slot

import (
	"errors"
	"slices"
)

// The record form of a graph line (card GRAPH-SLOT1, record scope v4):
// Record builds it from the bound line and validateKeyed checks it, split
// from graph.go for Law 10. It holds a path as its keyed digest and each
// list as its count, never a path or a name.

// Record is l's record form: every list as its count, the path and the
// source as their keyed digests (digest, intent's PathDigest), the knowledge
// ids, the title, the clause, a code line's role and basis, a unit line's
// basis and the kinds it or a not-known line names as not known, and
// every line's band kept, since those are the repository's declared
// records and the engine's reading, never the request's words. It is
// built from the bound line, so the counts are what the printed line
// listed, left out and cut.
func (l GraphLine) Record(digest func(string) string) GraphLine {
	out := GraphLine{keyed: true}
	m := l.more
	switch {
	case l.code != nil:
		c := *l.code
		m.Names += len(c.Names)
		m.Consts += len(c.Consts)
		m.Calls += len(c.Calls)
		m.Writes += len(c.Writes)
		m.Reads += len(c.Reads)
		m.Imports += len(c.Imports)
		out.digest = digest(string(c.Path))
		out.code = &CodeFact{Role: c.Role, Basis: c.Basis, Band: c.Band, Lessons: slices.Clone(c.Lessons)}
	case l.table != nil:
		t := *l.table
		m.Writers += len(t.Writers)
		m.Readers += len(t.Readers)
		out.table = &TableFact{Lessons: slices.Clone(t.Lessons)}
	case l.know != nil:
		k := *l.know
		if k.Path != "" {
			out.digest = digest(string(k.Path))
		}
		if k.Source != "" {
			out.sourceDigest = digest(string(k.Source))
		}
		out.know = &KnowledgeFact{Kind: k.Kind, ID: k.ID, Band: k.Band, Title: k.Title, Clause: k.Clause}
	case l.unit != nil:
		u := *l.unit
		m.Names += len(u.Names)
		m.Consumers += len(u.Consumers)
		out.digest = digest(string(u.Path))
		out.unit = &UnitFact{Basis: u.Basis, Band: u.Band, Lessons: slices.Clone(u.Lessons), Unknown: slices.Clone(u.Unknown)}
	case l.unknown != nil:
		out.unknown = &NotKnownFact{Unknown: slices.Clone(l.unknown.Unknown)}
	default:
		return GraphLine{}
	}
	out.more = m
	return out
}

// validateKeyed checks a record form: digests 64 lowercase hex, no path
// and no name on any fact, and every word in its set.
func (l GraphLine) validateKeyed() error {
	for _, d := range []string{l.digest, l.sourceDigest} {
		if d != "" && !digestShaped(d) {
			return errors.New("slot: graph line digest is not 64 lowercase hex")
		}
	}
	switch {
	case l.code != nil:
		c := *l.code
		if c.Path != "" || len(c.Names)+len(c.Consts)+len(c.Calls)+len(c.Writes)+len(c.Reads)+len(c.Imports) > 0 {
			return errors.New("slot: keyed graph line holds a path or a name")
		}
		if err := validRole(c.Role); err != nil {
			return err
		}
		if err := validBasis(c.Role, c.Basis); err != nil {
			return err
		}
		if err := validBand(c.Band); err != nil {
			return err
		}
		return validIDs(c.Lessons)
	case l.table != nil:
		t := *l.table
		if t.Table != "" || len(t.Writers)+len(t.Readers) > 0 {
			return errors.New("slot: keyed graph line holds a table or a name")
		}
		return validIDs(t.Lessons)
	case l.know != nil:
		k := *l.know
		if k.Path != "" || k.Source != "" {
			return errors.New("slot: keyed graph line holds a path")
		}
		return k.validate()
	case l.unit != nil:
		u := *l.unit
		if u.Path != "" || u.RunnerUp != "" || len(u.Names)+len(u.Consumers) > 0 {
			return errors.New("slot: keyed graph line holds a path or a name")
		}
		if err := u.validDisclosure(); err != nil {
			return err
		}
		return validIDs(u.Lessons)
	case l.unknown != nil:
		return l.unknown.validate()
	}
	return nil
}
