package slot

import (
	"strings"
	"testing"
)

// rebuilt is l's fact through its kind's exported constructor, the way a
// reader of a stored line makes it again: every list the bound line kept,
// and none of the counts bind left in more.
func rebuilt(l GraphLine) GraphLine {
	if f, ok := l.Code(); ok {
		return NewCodeLine(f)
	}
	if f, ok := l.Table(); ok {
		return NewTableLine(f)
	}
	if f, ok := l.Knowledge(); ok {
		return NewKnowledgeLine(f)
	}
	if f, ok := l.Unit(); ok {
		return NewUnitLine(f)
	}
	if f, ok := l.NotKnown(); ok {
		return NewNotKnownLine(f)
	}
	return GraphLine{}
}

// TestGraphLine_WithMoreRebindsToTheSameLine (W-M2.3 hook half, D10): a
// bound line rebuilt from its fact through its constructor, with the bound
// line's counts restored by WithMore, binds again to the same line: the
// same text, the same counts and the same record form. Rebinding is
// idempotent because every item bind kept passes its check and sits within
// its cap, so bind adds nothing to the counts it is given. Witness: rebuilt
// without WithMore, every line that bind cut renders differently, and a
// record claim that only counts its record is refused, so the test reads
// the counts and not only the lists. Break: make WithMore return l
// unchanged, and every cut case fails.
func TestGraphLine_WithMoreRebindsToTheSameLine(t *testing.T) {
	digest := func(s string) string { return strings.Repeat("cd", 32) }
	ids := func(n int, prefix string) []KnowledgeID {
		out := make([]KnowledgeID, n)
		for i := range out {
			out[i] = KnowledgeID(prefix + string(rune('A'+i)))
		}
		return out
	}
	names := func(n int) []Ident {
		out := make([]Ident, n)
		for i := range out {
			out[i] = Ident("n_" + string(rune('a'+i)))
		}
		return out
	}
	for _, c := range []struct {
		name string
		line GraphLine
		cut  bool
	}{
		{"code over every cap", NewCodeLine(CodeFact{Path: "jobs/purge_sessions.py", Role: RoleConsumer,
			Names: names(5), Calls: names(4),
			Consts:  []ConstFact{{Name: "A", Value: 1}, {Name: "B", Value: 2}, {Name: "C", Value: 3}},
			Writes:  []TableRef{{Table: "t1"}, {Table: "t2"}, {Table: "t3"}},
			Reads:   []TableRef{{Table: "r1"}, {Table: "r2"}, {Table: "r3"}},
			Imports: names(3), Lessons: ids(5, "les:L")}), true},
		{"code with an item its check refuses", NewCodeLine(CodeFact{Path: "a.go", Names: []Ident{"ok", "not ok"}}), true},
		{"table over every cap", NewTableLine(TableFact{Table: "public.sessions", Writers: names(4), Readers: names(5),
			Lessons: ids(4, "les:T")}), true},
		{"knowledge, nothing to cut", NewKnowledgeLine(inc212()), false},
		{"unit over every cap", NewUnitLine(UnitFact{Path: "internal/steer", Basis: BasisNamed, Names: names(5),
			Consumers: []RelPath{"a/x.go", "b/y.go", "c/z.go", "d/w.go"}, Lessons: ids(4, "dec:D")}), true},
		{"record claim over its cap", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: ids(5, "dec:R"), Units: 1}), true},
		{"record claim that only counts", GraphLine{unit: &UnitFact{Path: "x", Basis: BasisRecord, Units: 1},
			more: Counts{Records: 1}}, true},
		{"not known, nothing to cut", NewNotKnownLine(NotKnownFact{Unknown: []string{UnknownUnit, UnknownTarget}}), false},
	} {
		v, err := Bind(c.line)
		if err != nil {
			t.Fatalf("%s: Bind: %v", c.name, err)
		}
		bound := v.(GraphLine)
		if cut := bound.More() != (Counts{}); cut != c.cut {
			t.Fatalf("%s: the case's own premise: bound counts %+v, want cut %v", c.name, bound.More(), c.cut)
		}
		rv, err := Bind(rebuilt(bound).WithMore(bound.More()))
		if err != nil {
			t.Fatalf("%s: rebound: %v", c.name, err)
		}
		again := rv.(GraphLine)
		if got, want := again.Text(), bound.Text(); got != want {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, got, want)
		}
		if got, want := again.More(), bound.More(); got != want {
			t.Errorf("%s: counts %+v, want %+v", c.name, got, want)
		}
		if got, want := again.Record(digest).Text(), bound.Record(digest).Text(); got != want {
			t.Errorf("%s: record\n got %q\nwant %q", c.name, got, want)
		}
		if !c.cut {
			continue
		}
		bare, err := Bind(rebuilt(bound))
		if err == nil && bare.Text() == bound.Text() {
			t.Errorf("%s: witness: rebuilt without its counts still renders %q", c.name, bare.Text())
		}
	}
}
