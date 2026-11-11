package slot

import (
	"strings"
	"testing"
)

// TestGU7_ARecordClaimSaysTheRequestNamesItsRecords (W-M1.41b): a unit a
// record the request names settled is claimed by the record, so its line
// names the records and never reads as named in the request: in one unit
// it says the records apply here; in several it says how many, by what
// the ladder ranked this one first among them and the next one it
// ranked. When the request bars units the records also apply in, each
// count stays true of the records and no barred unit is named: one unit
// left says here and how many it bars, several left say the request does
// not bar them. The ids are listed to the cap and the rest counted, a
// record that fails its check among them; a candidate says so first. On a
// record claim's line the knowledge kind not known is the other records,
// so no line names a record as applying and then the records that apply
// as not known.
func TestGU7_ARecordClaimSaysTheRequestNamesItsRecords(t *testing.T) {
	for _, c := range []struct {
		name string
		line GraphLine
		want string
	}{
		{"one record, one unit", NewUnitLine(UnitFact{Path: "internal/steer", Basis: BasisRecord, Band: BandStated,
			Records: []KnowledgeID{"dec:D124"}, Units: 1}),
			"unit 'internal/steer' (the request names D124, which applies here)"},
		{"two records, one unit", NewUnitLine(UnitFact{Path: "gamma", Basis: BasisRecord, Records: []KnowledgeID{"les:B3", "dec:D2"}, Units: 1}),
			"unit 'gamma' (the request names B3 and D2, which apply here)"},
		{"several units, by word coverage", NewUnitLine(UnitFact{Path: "internal/steer", Basis: BasisRecord, Band: BandCandidate,
			Records: []KnowledgeID{"dec:D176", "dec:D109"}, Units: 3, First: BasisCoverage, RunnerUp: "internal/spend",
			Unknown: []string{UnknownTarget, UnknownReuse}}),
			"unit 'internal/steer' (a candidate: the request names D176 and D109, which apply in 3 units; this one by word coverage; runner-up 'internal/spend'); not known: the file to change, the code to reuse"},
		{"several units, by its name", NewUnitLine(UnitFact{Path: "internal/hook", Basis: BasisRecord,
			Records: []KnowledgeID{"dec:D1", "dec:D2", "les:B3"}, Units: 2, First: BasisUnitName, Names: []Ident{"Fire"}}),
			"unit 'internal/hook' (the request names D1, D2 and B3, which apply in 2 units; this one by its name) declares 'Fire'"},
		{"one unit left of two", NewUnitLine(UnitFact{Path: "internal/ledger", Basis: BasisRecord, Band: BandStated,
			Records: []KnowledgeID{"dec:D176"}, Units: 1, Barred: 1}),
			"unit 'internal/ledger' (the request names D176, which applies here and in 1 unit the request bars)"},
		{"one unit left of three", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: []KnowledgeID{"dec:D1", "dec:D2"}, Units: 1, Barred: 2}),
			"unit 'x' (the request names D1 and D2, which apply here and in 2 units the request bars)"},
		{"two units left of three", NewUnitLine(UnitFact{Path: "internal/ledger", Basis: BasisRecord, Band: BandCandidate,
			Records: []KnowledgeID{"dec:D190"}, Units: 2, Barred: 1, First: BasisCoverage, RunnerUp: "internal/report"}),
			"unit 'internal/ledger' (a candidate: the request names D190, which applies in 2 units the request does not bar; this one by word coverage; runner-up 'internal/report')"},
		{"over the cap", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord,
			Records: []KnowledgeID{"dec:D1", "dec:D2", "dec:D3", "dec:D4", "dec:D5"}, Units: 1}),
			"unit 'x' (the request names D1, D2, D3 and 2 more, which apply here)"},
		{"a record that fails its check", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: []KnowledgeID{"dec:D1", "dec:D 2"}, Units: 1}),
			"unit 'x' (the request names D1 and 1 more, which apply here)"},
		{"one record counted", GraphLine{unit: &UnitFact{Path: "x", Basis: BasisRecord, Units: 1}, more: Counts{Records: 1}},
			"unit 'x' (the request names 1 record, which applies here)"},
		{"records counted", GraphLine{unit: &UnitFact{Path: "x", Basis: BasisRecord, Units: 2, First: BasisCoverage}, more: Counts{Records: 2}},
			"unit 'x' (the request names 2 records, which apply in 2 units; this one by word coverage)"},
		{"other records not known", NewUnitLine(UnitFact{Path: "internal/steer", Basis: BasisRecord, Band: BandCandidate,
			Records: []KnowledgeID{"dec:D176"}, Units: 3, First: BasisCoverage, Unknown: []string{UnknownTarget, UnknownKnowledge}}),
			"unit 'internal/steer' (a candidate: the request names D176, which applies in 3 units; this one by word coverage); not known: the file to change, other records that apply to the change"},
		{"the records not known off a record claim", NewUnitLine(UnitFact{Path: "internal/steer", Basis: BasisCoverage, Band: BandCandidate,
			Unknown: []string{UnknownKnowledge}}),
			"unit 'internal/steer' (a candidate by word coverage); not known: the records that apply to the change"},
	} {
		v, err := Bind(c.line)
		if err != nil {
			t.Fatalf("%s: Bind: %v", c.name, err)
		}
		if got := v.Text(); got != c.want {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, got, c.want)
		}
		if err := v.(GraphLine).Validate(); err != nil {
			t.Errorf("%s: the bound line fails its own check: %v", c.name, err)
		}
	}
}

// TestGU7_ARecordClaimsFieldsHoldOrTheLineIsRefused (W-M1.41b): a record
// claim names at least one record, listed or counted, in at least one
// unit the request does not bar, counts the units it bars in the range a
// count holds, and ranks its unit by its name or by word coverage exactly
// when its records apply in more than one, the only claim that names a
// runner-up among them; a claim on any other basis carries none of its
// fields. Each failure refuses the line. A record that fails its check is
// refused by the line's own check, as a lesson is, and left out and
// counted at bind.
func TestGU7_ARecordClaimsFieldsHoldOrTheLineIsRefused(t *testing.T) {
	d1 := []KnowledgeID{"dec:D1"}
	for _, c := range []struct {
		name string
		line GraphLine
	}{
		{"records on a coverage claim", NewUnitLine(UnitFact{Path: "x", Basis: BasisCoverage, Records: d1})},
		{"a unit count on a named claim", NewUnitLine(UnitFact{Path: "x", Basis: BasisNamed, Units: 1})},
		{"a barred count on a coverage claim", NewUnitLine(UnitFact{Path: "x", Basis: BasisCoverage, Band: BandCandidate, Barred: 1})},
		{"a rank on a claim with no basis", NewUnitLine(UnitFact{Path: "x", First: BasisCoverage})},
		{"a counted record on a claim by the unit's name", GraphLine{unit: &UnitFact{Path: "x", Basis: BasisUnitName}, more: Counts{Records: 1}}},
		{"a record claim that names no record", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Units: 1})},
		{"a record claim in no unit", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1})},
		{"a record claim in too many units", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: maxCount + 1, First: BasisCoverage})},
		{"a negative barred count", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 1, Barred: -1})},
		{"too many barred units", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 1, Barred: maxCount + 1})},
		{"several units and no rank", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 2})},
		{"one unit and a rank", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 1, First: BasisCoverage})},
		{"a rank by a name the request held", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 2, First: BasisNamed})},
		{"a rank by a record", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 2, First: BasisRecord})},
		{"a runner-up in one unit", NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: d1, Units: 1, RunnerUp: "y"})},
	} {
		if _, err := Bind(c.line); err == nil {
			t.Errorf("%s: bound", c.name)
		}
		if err := c.line.Validate(); err == nil {
			t.Errorf("%s: passes the line's check", c.name)
		}
	}
	bad := NewUnitLine(UnitFact{Path: "x", Basis: BasisRecord, Records: []KnowledgeID{"dec:D1", "dec:D 2"}, Units: 1})
	if err := bad.Validate(); err == nil {
		t.Error("a record that fails its check passes the line's check")
	}
	v, err := Bind(bad)
	if err != nil {
		t.Fatalf("a record that fails its check refused the line at bind: %v", err)
	}
	if u, _ := v.(GraphLine).Unit(); len(u.Records) != 1 || v.(GraphLine).More().Records != 1 {
		t.Errorf("bound records %v, %d counted; want D1 and 1 counted", u.Records, v.(GraphLine).More().Records)
	}
}

// TestGU7_TheRecordFormKeepsTheRecordClaim (W-M1.41b, ruling iv): the
// record form of a record claim's line keeps the basis, the record ids,
// the count of those left out, the unit counts and the rank, and never the
// path or the runner-up's; it binds, and a record form holding a record
// that fails its check is refused.
func TestGU7_TheRecordFormKeepsTheRecordClaim(t *testing.T) {
	digest := func(string) string { return strings.Repeat("ab", 32) }
	v, err := Bind(NewUnitLine(UnitFact{Path: "internal/steer", Basis: BasisRecord, Band: BandCandidate,
		Records: []KnowledgeID{"dec:D176", "dec:D109", "dec:D1", "dec:D2"}, Units: 3, Barred: 1, First: BasisCoverage, RunnerUp: "internal/spend",
		Unknown: []string{UnknownTarget}}))
	if err != nil {
		t.Fatal(err)
	}
	rec := v.(GraphLine).Record(digest)
	u, ok := rec.Unit()
	if !ok || u.Path != "" || u.RunnerUp != "" || u.Basis != BasisRecord || u.Band != BandCandidate || u.Units != 3 || u.Barred != 1 || u.First != BasisCoverage ||
		strings.Join(idTexts(u.Records), ",") != "D176,D109,D1" || rec.More().Records != 1 {
		t.Errorf("record form unit %+v counts %+v", u, rec.More())
	}
	rv, err := Bind(rec)
	if err != nil {
		t.Fatalf("Bind(record): %v", err)
	}
	if want := "a unit on record; not known: the file to change"; rv.Text() != want {
		t.Errorf("record text\n got %q\nwant %q", rv.Text(), want)
	}
	if _, err := Bind(GraphLine{keyed: true, unit: &UnitFact{Basis: BasisRecord, Records: []KnowledgeID{"dec:D 1"}, Units: 1}}); err == nil {
		t.Error("a record form holding a record that fails its check was bound")
	}
	if _, err := Bind(GraphLine{keyed: true, unit: &UnitFact{Basis: BasisCoverage, Records: []KnowledgeID{"dec:D1"}}}); err == nil {
		t.Error("a record form holding records on a coverage claim was bound")
	}
	if _, err := Bind(GraphLine{keyed: true, unit: &UnitFact{Basis: BasisRecord, Records: []KnowledgeID{"dec:D1"}, Units: 1, Barred: -1}}); err == nil {
		t.Error("a record form holding a negative barred count was bound")
	}
}

func idTexts(ids []KnowledgeID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.Text()
	}
	return out
}
