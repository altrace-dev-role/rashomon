package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// stateAll states every cell: a test of how the roles become lines reads
// them before the calibration bands them.
func stateAll(string, string) string { return BandStated }

// TestBands_TheCalibrationBandsEachCellAsRuled (PLAN W-M1.38): the
// calibration's cells give the table the ruling fixed, a role line capped
// at its unit line's band (a consumer under a unit claimed by its name is
// stated by its own cell, 62 of 76, and a candidate by the cap); the
// floors are inclusive; the brief with no unit claim is withheld by its
// own cell (7 of 55) and a cell the calibration lacks states nothing; and
// the engine's role words are the slot's words for what is not known, so
// a unit line naming them binds.
func TestBands_TheCalibrationBandsEachCellAsRuled(t *testing.T) {
	kinds := []string{RoleTarget, RoleReuse, RoleConsumer, RoleKnowledge}
	for _, c := range []struct {
		basis string
		want  [5]string // unit, target, reuse, consumer, knowledge
	}{
		{BasisNamed, [5]string{BandStated, BandStated, BandWithheld, BandStated, BandStated}},
		{BasisUnitName, [5]string{BandCandidate, BandWithheld, BandWithheld, BandCandidate, BandCandidate}},
		{BasisCoverage, [5]string{BandCandidate, BandWithheld, BandWithheld, BandCandidate, BandWithheld}},
	} {
		unit := calibrated(RoleUnit, c.basis)
		got := [5]string{unit}
		for i, k := range kinds {
			got[i+1] = lower(calibrated(k, c.basis), unit)
		}
		if got != c.want {
			t.Errorf("basis %s: bands %v, want %v", c.basis, got, c.want)
		}
	}
	if b := calibrated(RoleConsumer, BasisUnitName); b != BandStated {
		t.Errorf("consumer under unit-name by its own cell: %s, want stated", b)
	}
	for _, c := range []struct {
		t    tally
		want string
	}{
		{tally{4, 5}, BandStated}, {tally{8, 10}, BandStated}, {tally{79, 100}, BandCandidate},
		{tally{1, 2}, BandCandidate}, {tally{49, 100}, BandWithheld}, {tally{0, 6}, BandWithheld}, {tally{0, 0}, BandWithheld},
	} {
		if got := c.t.band(); got != c.want {
			t.Errorf("tally %d of %d: %s, want %s", c.t.right, c.t.judged, got, c.want)
		}
	}
	if c := calibration[[2]string{RoleUnit, BasisNone}]; c != (tally{7, 55}) || calibrated(RoleUnit, BasisNone) != BandWithheld {
		t.Errorf("the brief with no unit claim: %d of %d, %s", c.right, c.judged, calibrated(RoleUnit, BasisNone))
	}
	if b := calibrated(RoleUnit, "guessed"); b != BandWithheld {
		t.Errorf("a cell the calibration lacks: %s", b)
	}
	if lower(BandStated, BandCandidate) != BandCandidate || lower(BandCandidate, BandWithheld) != BandWithheld || lower(BandStated, BandStated) != BandStated {
		t.Error("lower is not the less certain band")
	}
	for _, w := range [][2]string{{RoleTarget, slot.UnknownTarget}, {RoleReuse, slot.UnknownReuse}, {RoleConsumer, slot.UnknownConsumer}, {RoleKnowledge, slot.UnknownKnowledge}} {
		if w[0] != w[1] {
			t.Errorf("role %q is %q in the slot", w[0], w[1])
		}
	}
	if _, err := slot.Bind(slot.NewUnitLine(slot.UnitFact{Path: "svc", Unknown: kinds})); err != nil {
		t.Errorf("a unit line naming every role kind: %v", err)
	}
}

// TestBands_AWithheldLineLeavesItsSlotAndSaysWhy: withhold bands the
// merged lines and takes the withheld out with no line moving up into
// them; the unit line names the kinds taken out; a decision on a line
// taken out says so and the rest are renumbered; a knowledge line found
// through a file taken out is anchored to the unit; a related line keeps
// no band; a unit withheld takes every role line and leaves the related
// lines, anchored to nothing, behind a not-known line in the unit's slot
// that names the unit and the kinds taken out; a unit the request names
// no unit for (Roles.Unnamed) is withheld whatever its cell, and its
// decision says so.
func TestBands_AWithheldLineLeavesItsSlotAndSaysWhy(t *testing.T) {
	unit, target, reuse, cons := graph.ID("unit:py:svc"), graph.ID("file:svc/a.py"), graph.ID("file:svc/b.py"), graph.ID("file:jobs/c.py")
	lines := func() []Line {
		return []Line{
			{Key: unit, Role: RoleUnit, Basis: BasisCoverage},
			{Key: target, Role: RoleTarget},
			{Key: reuse, Role: RoleReuse, Basis: ReuseByCall},
			{Key: cons, Role: RoleConsumer},
			{Key: "les:INC-1", Role: RoleKnowledge, Anchor: target},
			{Key: "sym:py:svc:func:svc.a.f", Anchor: target},
		}
	}
	decisions := func() []Decision {
		return []Decision{
			{ID: unit, Line: 1, Why: "role: unit"}, {ID: target, Line: 2, Why: "role: target"}, {ID: reuse, Line: 3, Why: "role: reuse"},
			{ID: cons, Line: 4, Why: "role: consumer"}, {ID: "les:INC-1", Line: 5, Why: "role: knowledge"},
			{ID: "sym:py:svc:func:svc.a.f", Line: 6, Why: "related"}, {ID: "file:svc/z.py", Why: "cut: budget (role lines)"},
		}
	}
	r := Roles{Unit: unit, Basis: BasisCoverage}
	kept, ds := withhold(lines(), decisions(), r, calibrated)
	if len(kept) != 3 || kept[0].Key != unit || kept[1].Key != cons || kept[2].Role != "" {
		t.Fatalf("kept %+v", kept)
	}
	if kept[0].Band != BandCandidate || kept[1].Band != BandCandidate || kept[2].Band != "" {
		t.Errorf("bands %q %q %q", kept[0].Band, kept[1].Band, kept[2].Band)
	}
	if !slices.Equal(kept[0].Unknown, []string{RoleTarget, RoleReuse, RoleKnowledge}) || kept[1].Unknown != nil {
		t.Errorf("unknown %v on the unit line, %v on the consumer", kept[0].Unknown, kept[1].Unknown)
	}
	if kept[2].Anchor != unit {
		t.Errorf("a line found through the withheld target is anchored to %q", kept[2].Anchor)
	}
	want := []Decision{
		{ID: unit, Line: 1, Why: "role: unit"}, {ID: target, Why: "withheld: role target"}, {ID: reuse, Why: "withheld: role reuse"},
		{ID: cons, Line: 2, Why: "role: consumer"}, {ID: "les:INC-1", Why: "withheld: role knowledge"},
		{ID: "sym:py:svc:func:svc.a.f", Line: 3, Why: "related"}, {ID: "file:svc/z.py", Why: "cut: budget (role lines)"},
	}
	if !slices.Equal(ds, want) {
		t.Errorf("decisions\n got %+v\nwant %+v", ds, want)
	}
	r2 := Roles{Unit: unit, Basis: BasisUnitName}
	l2 := lines()
	l2[0].Basis = BasisUnitName
	kept, _ = withhold(l2, decisions(), r2, calibrated)
	if len(kept) != 4 || kept[1].Key != cons || kept[1].Band != BandCandidate || kept[2].Key != "les:INC-1" || kept[2].Band != BandCandidate {
		t.Errorf("unit-name: a consumer stated by its cell is a candidate under its candidate unit: %+v", kept)
	}
	withholdTarget := func(kind, basis string) string {
		if kind == RoleTarget {
			return BandWithheld
		}
		return BandStated
	}
	kept, _ = withhold(lines(), decisions(), r, withholdTarget)
	if len(kept) != 5 || kept[3].Key != "les:INC-1" || kept[3].Anchor != unit || kept[3].Band != BandStated {
		t.Errorf("knowledge found through the withheld target: %+v", kept)
	}
	withholdUnit := func(kind, basis string) string {
		if kind == RoleUnit {
			return BandWithheld
		}
		return BandStated
	}
	all := []string{slot.UnknownUnit, RoleTarget, RoleReuse, RoleConsumer, RoleKnowledge}
	kept, ds = withhold(lines(), decisions(), r, withholdUnit)
	if len(kept) != 2 || kept[0].Role != RoleNotKnown || kept[0].Key != "" || !slices.Equal(kept[0].Unknown, all) ||
		kept[1].Role != "" || kept[1].Anchor != "" || ds[0] != (Decision{ID: unit, Why: "withheld: role unit"}) || ds[5].Line != 2 {
		t.Errorf("unit withheld: kept %+v decisions %+v", kept, ds)
	}
	kept, ds = withhold(lines(), decisions(), Roles{Unit: unit, Basis: BasisNamed, Unnamed: true}, stateAll)
	if len(kept) != 2 || kept[0].Role != RoleNotKnown || !slices.Equal(kept[0].Unknown, all) || kept[1].Role != "" ||
		ds[0] != (Decision{ID: unit, Why: "withheld: role unit: the request names no unit"}) || ds[1] != (Decision{ID: target, Why: "withheld: role target"}) || ds[5].Line != 2 {
		t.Errorf("the request names no unit: kept %+v decisions %+v", kept, ds)
	}
	kept, ds = withhold(lines(), decisions(), r, stateAll)
	if len(kept) != 6 || kept[0].Unknown != nil || !slices.Equal(ds, decisions()) {
		t.Errorf("every cell stated: kept %d unknown %v decisions %+v", len(kept), kept[0].Unknown, ds)
	}
}

// TestBands_TaskAIsACandidateUnitAndItsConsumer: under the calibration
// Task A's unit, claimed by word coverage, is a candidate beside its
// runner-up; its consumer is a candidate; the target file, the reuse and
// INC-212 fall below the floor, so the unit line says they are not known
// and no related line moves up into their slots; the record form keeps
// the bands and the kinds not known.
func TestBands_TaskAIsACandidateUnitAndItsConsumer(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	r := b.Roles
	if r.Basis != BasisCoverage || r.File == "" || len(r.Reuse) == 0 || len(r.Consumers) == 0 || len(r.Knowledge) == 0 || len(r.Picks) < 2 {
		t.Fatalf("roles %+v", r)
	}
	if b.Kept != 2 || len(b.Lines) != 2 || b.Lines[0].Role != RoleUnit || b.Lines[1].Role != RoleConsumer {
		t.Fatalf("kept %d of %d lines %+v", b.Kept, len(b.Lines), b.Lines)
	}
	lines := strings.Split(b.Text(), "\n")[1:]
	head := "unit 'services/auth' (a candidate by word coverage; runner-up '" + r.Picks[1].Path + "') declares 'is_expired'"
	tail := "; not known: the file to change, the code to reuse, the records that apply to the change"
	if !strings.HasPrefix(lines[0], head) || !strings.HasSuffix(lines[0], tail) {
		t.Errorf("unit line\n got %q\nwant %q ... %q", lines[0], head, tail)
	}
	if want := "consumer 'jobs/purge_sessions.py' (a candidate)"; !strings.HasPrefix(lines[1], want) {
		t.Errorf("consumer line\n got %q\nwant prefix %q", lines[1], want)
	}
	// INC-212's own line is withheld (knowledge under a coverage unit); the
	// consumer line still lists it, a fact of that file.
	if strings.Contains(b.Text(), "lesson INC-212") || !strings.HasSuffix(lines[1], "; INC-212 applies") {
		t.Errorf("INC-212: its line withheld, the consumer's clause kept:\n%s", b.Text())
	}
	why := map[graph.ID]Decision{}
	for _, d := range b.Decisions {
		if _, ok := why[d.ID]; !ok {
			why[d.ID] = d
		}
	}
	for id, w := range map[graph.ID]string{r.File: "withheld: role target", r.Reuse[0].File: "withheld: role reuse", r.Knowledge[0]: "withheld: role knowledge"} {
		if d := why[id]; d.Why != w || d.Line != 0 {
			t.Errorf("%s: decision %+v, want %q", id, d, w)
		}
	}
	if d := why[r.Consumers[0].File]; d.Line != 2 {
		t.Errorf("consumer decision %+v", d)
	}
	digest := func(string) string { return strings.Repeat("ab", 32) }
	m, err := b.Record(digest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Text(), "a unit on record") || !strings.Contains(m.Text(), tail) || !strings.Contains(m.Text(), "consumer a file on record (a candidate)") {
		t.Errorf("record form:\n%s", m.Text())
	}
}

// TestBands_ARecordFoundThroughAWithheldFileNamesTheUnit: a record found
// through the target file names that file while the target is delivered;
// with the target withheld it is anchored to the unit, so a record that
// applies within the unit names the unit, never the withheld file, nor
// the file outside the unit it also applies to; its line carries its
// band.
func TestBands_ARecordFoundThroughAWithheldFileNamesTheUnit(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	rev := graph.RevCommit(graphtest.Commit)
	sym := graphtest.Sym("services/auth", "auth.sessions.is_expired")
	if _, ok := sp.Store.Node(sym, rev); !ok {
		t.Fatalf("fixture lacks %s", sym)
	}
	les := graph.ID("les:INC-100")
	sp.Store.AssertNode(graph.Node{ID: les, Kind: "lesson", Tier: graph.Declared, Conf: 1.0, Rev: rev, Attrs: map[string]string{
		"title": "INC-100 a double clock read", "clause": "the check reads the wall clock once per call", "source": "incidents/INC-100.md"}})
	sp.Store.AssertEdge(graph.Edge{Src: les, Pred: graph.AppliesTo, Dst: sym, Tier: graph.Declared, Conf: 1.0, Rev: rev})
	sp.Store.AssertEdge(graph.Edge{Src: les, Pred: graph.AppliesTo, Dst: graph.FileID("jobs/purge_sessions.py"), Tier: graph.Declared, Conf: 1.0, Rev: rev})
	prompt := "make session expiry respect the org-level timeout override."
	for _, c := range []struct {
		name  string
		bands bandFunc
		path  string
		band  string
	}{
		{"target stated", stateAll, "services/auth/auth/sessions.py", BandStated},
		{"target withheld", func(kind, basis string) string {
			switch kind {
			case RoleTarget:
				return BandWithheld
			case RoleKnowledge:
				return BandCandidate
			}
			return BandStated
		}, "services/auth", BandCandidate},
	} {
		b := Build(sp.Store, rev, prompt, Options{IDF: true, bands: c.bands})
		t.Logf("%s:\n%s", c.name, b.Text())
		var k *Line
		for i := range b.Lines[:b.Kept] {
			if b.Lines[i].Key == les {
				k = &b.Lines[i]
			}
		}
		if k == nil || b.Roles.KnowledgeVia[les] != b.Roles.File {
			t.Fatalf("%s: INC-100 not delivered through the target file (via %q):\n%s", c.name, b.Roles.KnowledgeVia[les], b.Text())
		}
		if f, _ := k.Slot.Knowledge(); string(f.Path) != c.path || f.Band != c.band {
			t.Errorf("%s: INC-100 applies to %q in band %q, want %q in %q", c.name, f.Path, f.Band, c.path, c.band)
		}
	}
}

// TestBands_ABriefWithNoUnitClaimOpensWithTheNotKnownLine (W-M1.38): a
// brief whose roles settled on no unit is read by its first line's unit,
// a claim right on 7 of the 55 such briefs the calibration read, under
// the floor; so the brief opens with the not-known line naming the unit
// alone, the related lines follow it unbanded, the last one cut to the
// budget with a decision saying why and the rest renumbered. Where that
// cell is stated the brief is as before, and a brief with no line gets
// no not-known line.
func TestBands_ABriefWithNoUnitClaimOpensWithTheNotKnownLine(t *testing.T) {
	keys := []graph.ID{"file:svc/a.py", "file:svc/b.py", "tbl:public.t", "sym:py:svc:func:svc.c.f", "les:INC-1"}
	lines := func() []Line {
		out := make([]Line, len(keys))
		for i, k := range keys {
			out[i] = Line{Key: k}
		}
		return out
	}
	decisions := func() []Decision {
		out := []Decision{{ID: "file:svc/z.py", Why: "cut: budget (3 of 9)"}}
		for i, k := range keys {
			out = append(out, Decision{ID: k, Line: i + 1, Why: "related"})
		}
		return out
	}
	kept, ds := withhold(lines(), decisions(), Roles{}, calibrated)
	if len(kept) != MaxLines || kept[0].Role != RoleNotKnown || kept[0].Key != "" || !slices.Equal(kept[0].Unknown, []string{slot.UnknownUnit}) {
		t.Fatalf("kept %+v", kept)
	}
	for i, l := range kept[1:] {
		if l.Key != keys[i] || l.Role != "" || l.Band != "" {
			t.Errorf("line %d: %+v, want the related line %s unbanded", i+2, l, keys[i])
		}
	}
	want := []Decision{{ID: "file:svc/z.py", Why: "cut: budget (3 of 9)"}, {ID: keys[0], Line: 2, Why: "related"}, {ID: keys[1], Line: 3, Why: "related"},
		{ID: keys[2], Line: 4, Why: "related"}, {ID: keys[3], Line: 5, Why: "related"}, {ID: keys[4], Why: "cut: budget (not-known line)"}}
	if !slices.Equal(ds, want) {
		t.Errorf("decisions\n got %+v\nwant %+v", ds, want)
	}
	kept, ds = withhold(lines(), decisions(), Roles{}, stateAll)
	if len(kept) != len(keys) || kept[0].Key != keys[0] || !slices.Equal(ds, decisions()) {
		t.Errorf("the cell stated: kept %+v decisions %+v", kept, ds)
	}
	if kept, _ := withhold(nil, nil, Roles{}, calibrated); len(kept) != 0 {
		t.Errorf("a brief with no line: %+v", kept)
	}
}
