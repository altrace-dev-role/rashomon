package slot

import (
	"strings"
	"testing"
)

func platformUnit() GraphLine {
	return NewUnitLine(UnitFact{Path: "internal/platform", Names: []Ident{"Read", "Callers", "Locate", "LogPath"},
		Consumers: []RelPath{"cmd/rashomon/report_platform.go", "cmd/rashomon/blast.go", "cmd/rashomon/laststop.go"},
		Lessons:   []KnowledgeID{"dec:D145", "dec:D149-addendum"}})
}

// TestGU1_AUnitLineAndARoledCodeLineRenderAsOneSentenceEach: a unit line
// is its path, the names the request touches, the files outside it that
// consume them and the records that apply, each list cut at its cap; a
// code line with a role opens with the role's word; a requirement is a
// knowledge kind of its own.
func TestGU1_AUnitLineAndARoledCodeLineRenderAsOneSentenceEach(t *testing.T) {
	for _, c := range []struct {
		name string
		line GraphLine
		want string
		kind string
	}{
		{"unit", platformUnit(),
			"unit 'internal/platform' declares 'Read', 'Callers', 'Locate' and 1 more; is consumed by 'cmd/rashomon/report_platform.go', 'cmd/rashomon/blast.go' and 1 more; D145, D149-addendum apply", "unit"},
		{"bare unit", NewUnitLine(UnitFact{Path: "pgraph"}), "unit 'pgraph'", "unit"},
		{"reuse", NewCodeLine(CodeFact{Path: "internal/platform/callers.go", Role: RoleReuse, Names: []Ident{"Callers", "Locate"}}),
			"reuse 'internal/platform/callers.go' declares 'Callers', 'Locate'", "code"},
		{"consumer", NewCodeLine(CodeFact{Path: "cmd/rashomon/report_platform.go", Role: RoleConsumer, Names: []Ident{"platformReadWithin"}, Calls: []Ident{"Read", "LogPath"}}),
			"consumer 'cmd/rashomon/report_platform.go' declares 'platformReadWithin'; calls 'Read', 'LogPath'", "code"},
		{"requirement", NewKnowledgeLine(KnowledgeFact{Kind: KnowledgeRequirement, ID: "req:PF-PL11", Path: "internal/platform/read.go",
			Clause: "Unknown, never zero; status none; missing: fan-in itself (PF-PL7)", Source: "truth_table.json"}),
			"requirement PF-PL11 applies to 'internal/platform/read.go': 'Unknown, never zero; status none; missing: fan-in itself (PF-PL7)'; source 'truth_table.json'", "requirement"},
	} {
		v, err := Bind(c.line)
		if err != nil {
			t.Fatalf("%s: Bind: %v", c.name, err)
		}
		if got := v.Text(); got != c.want {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, got, c.want)
		}
		if got := v.(GraphLine); got.Kind() != c.kind || !got.Lists() {
			t.Errorf("%s: kind %q lists %v", c.name, got.Kind(), got.Lists())
		}
	}
}

// TestGU1_TheUnitRecordFormHoldsItsDigestCountsAndIds: the record form of
// a unit line keys the path, counts the names and consumers and keeps the
// ids; a code line's role survives into the record; a role outside the two
// words, a unit path that fails a check and a record form naming a path
// are refused; a consumer path that fails a check is left out and counted.
func TestGU1_TheUnitRecordFormHoldsItsDigestCountsAndIds(t *testing.T) {
	digest := func(string) string { return strings.Repeat("ab", 32) }
	v, err := Bind(platformUnit())
	if err != nil {
		t.Fatal(err)
	}
	r := v.(GraphLine).Record(digest)
	if !r.Keyed() || r.Digest() != strings.Repeat("ab", 32) || r.Kind() != "unit" {
		t.Fatalf("record = keyed %v digest %q kind %q", r.Keyed(), r.Digest(), r.Kind())
	}
	if m := r.More(); m != (Counts{Names: 4, Consumers: 3}) {
		t.Errorf("record counts = %+v", m)
	}
	if u, ok := r.Unit(); !ok || u.Path != "" || len(u.Names) != 0 || len(u.Consumers) != 0 || len(u.Lessons) != 2 {
		t.Errorf("record unit fact = %+v %v", u, ok)
	}
	rv, err := Bind(r)
	if err != nil {
		t.Fatalf("Bind(record): %v", err)
	}
	if want := "a unit on record declares 4 names; is consumed by 3 files; D145, D149-addendum apply"; rv.Text() != want {
		t.Errorf("record text\n got %q\nwant %q", rv.Text(), want)
	}
	cv, _ := Bind(NewCodeLine(CodeFact{Path: "a.go", Role: RoleConsumer, Names: []Ident{"f"}}))
	cr, _ := Bind(cv.(GraphLine).Record(digest))
	if want := "consumer a file on record declares 1 names"; cr.Text() != want {
		t.Errorf("roled record text = %q", cr.Text())
	}
	if _, err := Bind(NewCodeLine(CodeFact{Path: "a.go", Role: "target"})); err == nil {
		t.Error("a role outside reuse and consumer was bound")
	}
	if _, err := Bind(NewUnitLine(UnitFact{Path: "../../../../../far"})); err == nil {
		t.Error("a unit path climbing five directories was bound")
	}
	if _, err := Bind(GraphLine{keyed: true, unit: &UnitFact{Path: "internal/platform"}}); err == nil {
		t.Error("a record form holding a unit path was bound")
	}
	lv, err := Bind(NewUnitLine(UnitFact{Path: "internal/platform", Consumers: []RelPath{"ok.go", "/abs.go"}}))
	if err != nil {
		t.Fatal(err)
	}
	if got := lv.Text(); got != "unit 'internal/platform' is consumed by 'ok.go' and 1 more" {
		t.Errorf("a refused consumer: %q", got)
	}
	if _, err := Bind(GraphLine{code: &CodeFact{Path: "a.go"}, unit: &UnitFact{Path: "b"}}); err == nil {
		t.Error("a line holding a code and a unit fact was bound")
	}
}
