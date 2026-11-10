package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The record claim's own basis (W-M1.41b): a unit the records the request
// names settled is claimed on BasisRecord, and its line names those records
// and never reads as a unit the request named. recordRepo is names2's
// repository (TestNames_ARecordOutranksAClaimByWordCoverage): xray claims
// "the session purge timer" by one name, alpha and gamma by part of it and
// beta not at all; D1 applies in alpha and beta, D2 and B3 in gamma, B4 in
// xray, and D7 in no unit.
func recordRepo(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"xray", "xray/timer.py", []synSym{fn("session_purge_timer")}},
		{"alpha", "alpha/purge.py", []synSym{fn("session_purge")}},
		{"beta", "beta/notes.py", []synSym{fn("render_notes")}},
		{"gamma", "gamma/session.py", []synSym{fn("open_session")}},
	}, nil, nil)
	record(t, g, "dec:D1", "decision", "alpha/purge.py", "beta/notes.py")
	record(t, g, "dec:D2", "decision", "gamma/session.py")
	record(t, g, "les:B3", "lesson", "gamma/session.py")
	record(t, g, "les:B4", "lesson", "xray/timer.py")
	record(t, g, "dec:D7", "decision")
	return g
}

// recordBrief is the brief Build gives prompt over g under the unit rule,
// and its unit line's fact; a brief that delivers no unit line fails.
func recordBrief(t *testing.T, g *graph.Store, prompt string, rule unitRule) (Brief, slot.UnitFact) {
	t.Helper()
	b := Build(g, graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true, unitRule: rule})
	t.Logf("%q:\n%s\nbrief:\n%s", prompt, b.Explain(), b.Text())
	if b.Kept == 0 || b.Lines[0].Role != RoleUnit {
		t.Fatalf("%q: no unit line delivered", prompt)
	}
	u, _ := b.Lines[0].Slot.Unit()
	return b, u
}

func sameRecord(a, b RecordClaim) bool {
	return slices.Equal(a.IDs, b.IDs) && a.Units == b.Units && a.Barred == b.Barred && a.First == b.First && a.RunnerUp == b.RunnerUp
}

// TestRecord_RecordsInOneUnitAreStatedInTheNamedCell: records whose
// evidence lies in one unit claim it on the record basis, banded in the
// named cell, the reading a claim on one unit rests on (ruling (ii)), so
// under the shipped unit rule the line is stated and says the request
// names the records, which apply here, in the request's order; it names
// no runner-up, over a claim by coverage or with no words to rank. A
// named record whose evidence lies in no unit (D7) is not one of the
// claim's: the line never says it applies here.
func TestRecord_RecordsInOneUnitAreStatedInTheNamedCell(t *testing.T) {
	g := recordRepo(t)
	for _, c := range []struct {
		prompt string
		ids    []graph.ID
		head   string
	}{
		{"B3 again: fix the session purge timer.", []graph.ID{"les:B3"}, "unit 'gamma' (the request names B3, which applies here)"},
		{"B3 and D2 again.", []graph.ID{"les:B3", "dec:D2"}, "unit 'gamma' (the request names B3 and D2, which apply here)"},
		{"B3 and D7 again.", []graph.ID{"les:B3"}, "unit 'gamma' (the request names B3, which applies here)"},
	} {
		b, u := recordBrief(t, g, c.prompt, nil)
		r := b.Roles
		if want := (RecordClaim{IDs: c.ids, Units: 1}); r.Path != "gamma" || r.Basis != BasisRecord || !sameRecord(r.Record, want) || r.RunnerUp != "" || r.Unnamed {
			t.Errorf("%q: path %q basis %q record %+v runner-up %q unnamed %v, want gamma on %+v", c.prompt, r.Path, r.Basis, r.Record, r.RunnerUp, r.Unnamed, want)
		}
		if r.cell() != BasisNamed || b.Lines[0].Band != BandStated {
			t.Errorf("%q: cell %q band %q, want the named cell, stated", c.prompt, r.cell(), b.Lines[0].Band)
		}
		if u.Basis != BasisRecord || u.Units != 1 || u.First != "" || u.RunnerUp != "" || len(u.Records) != len(c.ids) {
			t.Errorf("%q: unit fact %+v", c.prompt, u)
		}
		if !strings.HasPrefix(b.Lines[0].Text, c.head) {
			t.Errorf("%q: unit line\n got %q\nwant prefix %q", c.prompt, b.Lines[0].Text, c.head)
		}
	}
}

// TestRecord_RecordsInSeveralUnitsAreBandedByTheLaddersReading: records
// whose evidence lies in three units claim alpha, the one the ladder ranks
// first among them, by part of the phrase over xray's claim by one name;
// the claim is banded in the coverage cell, the reading that ranked alpha,
// so it is a candidate, and its line says so, counts the units and names
// the runner-up among them: gamma, the next of the records' units the
// ladder ranked, never beta, listed before gamma among the records' units
// and on no rung of the ladder, nor alpha itself. The order the request
// names the records in orders the line's list alone.
func TestRecord_RecordsInSeveralUnitsAreBandedByTheLaddersReading(t *testing.T) {
	g := recordRepo(t)
	gamma := graph.UnitID(synLang, "gamma")
	for _, c := range []struct {
		prompt string
		ids    []graph.ID
		head   string
	}{
		{"D1 and D2 again: fix the session purge timer.", []graph.ID{"dec:D1", "dec:D2"},
			"unit 'alpha' (a candidate: the request names D1 and D2, which apply in 3 units; this one by word coverage; runner-up 'gamma')"},
		{"D2 and D1 again: fix the session purge timer.", []graph.ID{"dec:D2", "dec:D1"},
			"unit 'alpha' (a candidate: the request names D2 and D1, which apply in 3 units; this one by word coverage; runner-up 'gamma')"},
	} {
		b, u := recordBrief(t, g, c.prompt, nil)
		r := b.Roles
		want := RecordClaim{IDs: c.ids, Units: 3, First: BasisCoverage, RunnerUp: gamma}
		if r.Path != "alpha" || r.Basis != BasisRecord || !sameRecord(r.Record, want) || r.RunnerUp != gamma || r.Unnamed {
			t.Errorf("%q: path %q basis %q record %+v runner-up %q unnamed %v, want alpha on %+v", c.prompt, r.Path, r.Basis, r.Record, r.RunnerUp, r.Unnamed, want)
		}
		if r.cell() != BasisCoverage || b.Lines[0].Band != BandCandidate {
			t.Errorf("%q: cell %q band %q, want the coverage cell, a candidate", c.prompt, r.cell(), b.Lines[0].Band)
		}
		if u.Units != 3 || u.First != BasisCoverage || u.RunnerUp != "gamma" {
			t.Errorf("%q: unit fact %+v", c.prompt, u)
		}
		if !strings.HasPrefix(b.Lines[0].Text, c.head) {
			t.Errorf("%q: unit line\n got %q\nwant prefix %q", c.prompt, b.Lines[0].Text, c.head)
		}
	}
}

// TestRecord_ARecordClaimTheSlotRefusesIsWithheld: a record claim is
// banded in the cell of no claim, which withholds the unit line, exactly
// when the slot refuses the unit line its fields make (several units and
// no rank, a rank by name or by the records, a rank or a runner-up on one
// unit, no record, no unit); else in the named cell on one unit and the
// ranking reading's on several. A claim on another basis keeps its own.
func TestRecord_ARecordClaimTheSlotRefusesIsWithheld(t *testing.T) {
	g := recordRepo(t)
	gamma, xray := graph.UnitID(synLang, "gamma"), graph.UnitID(synLang, "xray")
	ids := []graph.ID{"dec:D2", "les:B3"}
	for _, c := range []struct {
		rc   RecordClaim
		want string
	}{
		{RecordClaim{IDs: ids, Units: 1}, BasisNamed},
		{RecordClaim{IDs: ids, Units: 3, First: BasisCoverage, RunnerUp: xray}, BasisCoverage},
		{RecordClaim{IDs: ids, Units: 2, First: BasisUnitName}, BasisUnitName},
		{RecordClaim{IDs: ids, Units: 2}, BasisNone},
		{RecordClaim{IDs: ids, Units: 2, First: BasisNamed}, BasisNone},
		{RecordClaim{IDs: ids, Units: 2, First: BasisRecord}, BasisNone},
		{RecordClaim{IDs: ids, Units: 1, First: BasisCoverage}, BasisNone},
		{RecordClaim{IDs: ids, Units: 1, RunnerUp: xray}, BasisNone},
		{RecordClaim{Units: 1}, BasisNone},
		{RecordClaim{IDs: ids, First: BasisCoverage}, BasisNone},
	} {
		l := Line{Role: RoleUnit, Key: gamma, Basis: BasisRecord, RunnerUp: c.rc.RunnerUp, Record: c.rc}
		err := slot.NewUnitLine(unitFact(g, graph.RevCommit(graphtest.Commit), l, func(p string) string { return p })).Validate()
		if got := (Roles{Basis: BasisRecord, Record: c.rc}).cell(); got != c.want || (got == BasisNone) != (err != nil) {
			t.Errorf("record %+v: cell %q, want %q; the slot's verdict on its unit line: %v", c.rc, got, c.want, err)
		}
	}
	for _, basis := range []string{BasisNamed, BasisUnitName, BasisCoverage, BasisNone} {
		if got := (Roles{Basis: basis}).cell(); got != basis {
			t.Errorf("a claim on %q is banded in the %q cell", basis, got)
		}
	}
	if b := calibrated(RoleUnit, BasisNone); b != BandWithheld {
		t.Errorf("a unit line in the cell of no claim is %q, want withheld", b)
	}
}

// TestRecord_AClaimTheRecordsDidNotSettleKeepsItsBasis: a qualified claim
// that covers its whole phrase stands against a named record whose
// evidence lies in another unit (B4 on xray against alpha, overRecord),
// and keeps its basis: the record's evidence lies in another unit, so it
// does not settle the claim (ladderStands, W-M1.41c). A request that
// names no record makes no record claim. Neither line says the request
// names a record.
func TestRecord_AClaimTheRecordsDidNotSettleKeepsItsBasis(t *testing.T) {
	g := recordRepo(t)
	for _, c := range []struct{ prompt, path string }{
		{"B4 again: fix the alpha session purge.", "alpha"},
		{"Fix the session purge timer.", "xray"},
	} {
		b, _ := recordBrief(t, g, c.prompt, noUnitRule)
		r := b.Roles
		if r.Path != c.path || r.Basis != BasisCoverage || r.Record.Units != 0 || r.Record.IDs != nil || r.cell() != BasisCoverage {
			t.Errorf("%q: path %q basis %q record %+v cell %q, want %s by coverage", c.prompt, r.Path, r.Basis, r.Record, r.cell(), c.path)
		}
		if strings.Contains(b.Text(), "the request names") {
			t.Errorf("%q: a claim by coverage reads as a record claim:\n%s", c.prompt, b.Text())
		}
	}
}

// TestRecord_AUnitLineNamesNoKnowledgeItsRecordsCover (MUST-FIX (A),
// KI14's shape): under a record claim banded in the coverage cell the
// target, the reuse and the knowledge lines fall below the floor. When
// every knowledge line withheld is one of the claim's records, which the
// unit line names as applying, the line names no knowledge kind as not
// known; when one is not (B9), it names the kind, and the slot says the
// records not known are other records; any other claim names the kind as
// before. A unit withheld leaves a not-known line, which names no record
// and so names every kind.
func TestRecord_AUnitLineNamesNoKnowledgeItsRecordsCover(t *testing.T) {
	unit, target, reuse, cons := graph.ID("unit:go:internal/report"), graph.ID("file:internal/report/a.go"), graph.ID("file:internal/report/b.go"), graph.ID("file:cmd/c.go")
	claim := RecordClaim{IDs: []graph.ID{"dec:D176", "dec:D109"}, Units: 3, First: BasisCoverage, RunnerUp: "unit:go:internal/spend"}
	byRecord := Roles{Unit: unit, Basis: BasisRecord, Record: claim}
	lines := func(r Roles, know ...graph.ID) []Line {
		ls := []Line{
			{Key: unit, Role: RoleUnit, Basis: r.Basis, Record: r.Record},
			{Key: target, Role: RoleTarget},
			{Key: reuse, Role: RoleReuse, Basis: ReuseByCall},
			{Key: cons, Role: RoleConsumer},
		}
		for _, k := range know {
			ls = append(ls, Line{Key: k, Role: RoleKnowledge, Anchor: target})
		}
		return ls
	}
	for _, c := range []struct {
		name string
		r    Roles
		know []graph.ID
		want []string
		tail string
	}{
		{"one of its records", byRecord, []graph.ID{"dec:D176"}, []string{RoleTarget, RoleReuse},
			"; not known: the file to change, the code to reuse"},
		{"both its records", byRecord, []graph.ID{"dec:D176", "dec:D109"}, []string{RoleTarget, RoleReuse},
			"; not known: the file to change, the code to reuse"},
		{"another record", byRecord, []graph.ID{"dec:D176", "les:B9"}, []string{RoleTarget, RoleReuse, RoleKnowledge},
			"; not known: the file to change, the code to reuse, other records that apply to the change"},
		{"a claim by coverage", Roles{Unit: unit, Basis: BasisCoverage}, []graph.ID{"dec:D176"}, []string{RoleTarget, RoleReuse, RoleKnowledge},
			"; not known: the file to change, the code to reuse, the records that apply to the change"},
	} {
		kept, _ := withhold(lines(c.r, c.know...), nil, c.r, calibrated)
		if len(kept) != 2 || kept[0].Role != RoleUnit || kept[0].Band != BandCandidate || kept[1].Role != RoleConsumer {
			t.Fatalf("%s: kept %+v, want the unit and the consumer, candidates", c.name, kept)
		}
		if !slices.Equal(kept[0].Unknown, c.want) {
			t.Errorf("%s: not known %v, want %v", c.name, kept[0].Unknown, c.want)
		}
		f := slot.UnitFact{Path: "internal/report", Basis: c.r.Basis, Band: kept[0].Band, Unknown: kept[0].Unknown}
		if c.r.Basis == BasisRecord {
			f.Records, f.Units, f.First, f.RunnerUp = appendKnowledge(nil, claim.IDs), claim.Units, claim.First, "internal/spend"
		}
		v, err := slot.Bind(slot.NewUnitLine(f))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.HasSuffix(v.Text(), c.tail) {
			t.Errorf("%s: unit line\n got %q\nwant suffix %q", c.name, v.Text(), c.tail)
		}
	}
	withholdUnit := func(kind, basis string) string {
		if kind == RoleUnit {
			return BandWithheld
		}
		return BandStated
	}
	kept, _ := withhold(lines(byRecord, "dec:D176"), nil, byRecord, withholdUnit)
	all := []string{slot.UnknownUnit, RoleTarget, RoleReuse, RoleConsumer, RoleKnowledge}
	if len(kept) != 1 || kept[0].Role != RoleNotKnown || !slices.Equal(kept[0].Unknown, all) {
		t.Errorf("a record claim's unit withheld: kept %+v, want one not-known line naming %v", kept, all)
	}
}

// TestRecord_E4KeepsARecordClaim: E4 withholds a claim not by name when
// the request holds neither the unit's path nor its last segment; a
// record claim is by the record the request names (a named claim when E4
// was registered), so E4 keeps it.
func TestRecord_E4KeepsARecordClaim(t *testing.T) {
	g := recordRepo(t)
	prompt := "B3 again: fix the session purge timer."
	b, _ := recordBrief(t, g, prompt, unitRuleE4)
	if r := b.Roles; r.Path != "gamma" || r.Basis != BasisRecord || r.Unnamed || unitRuleE4(corpus{}, r, prompt) {
		t.Errorf("path %q basis %q unnamed %v, want gamma by its record, kept by E4", r.Path, r.Basis, r.Unnamed)
	}
}

// TestRecord_ARecordClaimsNoUnitTheRequestBars: the records' claim honours
// the bar the ladder honours (barred, W-M1.34): B374 applies in
// internal/spend alone, D176 in internal/spend and internal/ledger. With
// nothing barred B374 claims internal/spend (the premise). Barred by an
// exclusion or a negated verb, B374 claims no unit, and never takes
// internal/ledger's claim by coverage over the ladder; D176 claims
// internal/ledger, the one unit left, and the line says it applies here
// and in 1 unit the request bars, naming none; a request naming both
// B374 and D176 claims internal/ledger on D176 alone, never saying B374
// applies there (W-M1.41b, the barred unit). The barred units counted are
// those of the claim's records alone: D200 lies in internal/ledger alone,
// so beside B374 its line counts none. Records in several units the bar
// leaves two of (D1 and D2 with beta barred) claim alpha as the ladder
// ranks it, and the line says the request does not bar the two.
func TestRecord_ARecordClaimsNoUnitTheRequestBars(t *testing.T) {
	g := synth(t, []synFile{
		{"internal/spend", "internal/spend/budget.py", []synSym{fn("spend_budget"), fn("cap_spend")}},
		{"internal/ledger", "internal/ledger/entries.py", []synSym{fn("entry_totals"), fn("post_entry")}},
		{"internal/report", "internal/report/render.py", []synSym{fn("render_report")}},
	}, nil, nil)
	record(t, g, "les:B374", "lesson", "internal/spend/budget.py")
	record(t, g, "dec:D176", "decision", "internal/spend/budget.py", "internal/ledger/entries.py")
	record(t, g, "dec:D200", "decision", "internal/ledger/entries.py")
	d176 := RecordClaim{IDs: []graph.ID{"dec:D176"}, Units: 1, Barred: 1}
	d176Head := "unit 'internal/ledger' (the request names D176, which applies here and in 1 unit the request bars)"
	for _, c := range []struct {
		prompt, path string
		rc           RecordClaim
		head         string
	}{
		{"B374 again", "internal/spend", RecordClaim{IDs: []graph.ID{"les:B374"}, Units: 1}, "unit 'internal/spend' (the request names B374, which applies here)"},
		{"B374 again, without touching internal/spend", "", RecordClaim{}, ""},
		{"B374 again; do not touch internal/spend", "", RecordClaim{}, ""},
		{"B374 again: fix the entry totals, without touching internal/spend", "internal/ledger", RecordClaim{}, ""},
		{"D176 again, without touching internal/spend", "internal/ledger", d176, d176Head},
		{"B374 and D176 again, without touching internal/spend", "internal/ledger", d176, d176Head},
		{"B374 and D200 again, without touching internal/spend", "internal/ledger", RecordClaim{IDs: []graph.ID{"dec:D200"}, Units: 1},
			"unit 'internal/ledger' (the request names D200, which applies here)"},
	} {
		b := Build(g, graph.RevCommit(graphtest.Commit), c.prompt, Options{IDF: true})
		r := b.Roles
		t.Logf("%q:\n%s\nbrief:\n%s", c.prompt, b.Explain(), b.Text())
		if r.Path != c.path || !sameRecord(r.Record, c.rc) || (r.Basis == BasisRecord) != (c.rc.Units > 0) {
			t.Errorf("%q: path %q basis %q record %+v, want %q on %+v", c.prompt, r.Path, r.Basis, r.Record, c.path, c.rc)
		}
		if c.head != "" && (b.Kept == 0 || !strings.HasPrefix(b.Lines[0].Text, c.head)) {
			t.Errorf("%q: brief\n%s\nwant its unit line to open %q", c.prompt, b.Text(), c.head)
		}
	}
	prompt := "D1 and D2 again: fix the session purge timer, without touching beta"
	b, u := recordBrief(t, recordRepo(t), prompt, nil)
	want := RecordClaim{IDs: []graph.ID{"dec:D1", "dec:D2"}, Units: 2, Barred: 1, First: BasisCoverage, RunnerUp: graph.UnitID(synLang, "gamma")}
	head := "unit 'alpha' (a candidate: the request names D1 and D2, which apply in 2 units the request does not bar; this one by word coverage; runner-up 'gamma')"
	if r := b.Roles; r.Path != "alpha" || !sameRecord(r.Record, want) || u.Units != 2 || u.Barred != 1 || !strings.HasPrefix(b.Lines[0].Text, head) {
		t.Errorf("%q: path %q record %+v unit fact %+v line %q, want alpha on %+v opening %q", prompt, r.Path, r.Record, u, b.Lines[0].Text, want, head)
	}
}
