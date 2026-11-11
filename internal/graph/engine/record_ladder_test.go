package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// heldRepo is a corpus whose ladder pick on "the session purge timer" is
// xray's timer and clock (W-M1.41c): B4 applies to the timer, B5 to the
// clock and B8 to the idle loop, which the pick does not reach; D6 applies
// to xray as a whole and B7 to the clock and to gamma as a whole; D1 to
// alpha's purge and beta's notes, D2 to gamma's session.
func heldRepo(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"xray", "xray/timer.py", []synSym{fn("session_purge_timer")}},
		{"xray", "xray/clock.py", []synSym{fn("purge_clock")}},
		{"xray", "xray/idle.py", []synSym{fn("idle_loop")}},
		{"alpha", "alpha/purge.py", []synSym{fn("session_purge")}},
		{"alpha", "alpha/notes.py", []synSym{fn("purge_notes")}},
		{"beta", "beta/notes.py", []synSym{fn("render_notes")}},
		{"gamma", "gamma/session.py", []synSym{fn("open_session")}},
	}, nil, nil)
	unitWide := func(id, unit string) {
		g.AssertEdge(graph.Edge{Src: graph.ID(id), Pred: graph.AppliesTo, Dst: graph.UnitID(synLang, unit), Tier: graph.Declared, Conf: 1, Rev: graph.RevCommit(graphtest.Commit)})
	}
	record(t, g, "les:B4", "lesson", "xray/timer.py")
	record(t, g, "les:B5", "lesson", "xray/clock.py")
	record(t, g, "les:B8", "lesson", "xray/idle.py")
	record(t, g, "dec:D1", "decision", "alpha/purge.py", "beta/notes.py")
	record(t, g, "dec:D2", "decision", "gamma/session.py")
	record(t, g, "dec:D6", "decision")
	unitWide("dec:D6", "xray")
	record(t, g, "les:B7", "lesson", "xray/clock.py")
	unitWide("les:B7", "gamma")
	return g
}

var (
	heldTimer = graph.SymID(synLang, "xray", "func", "timer.session_purge_timer")
	heldClock = graph.SymID(synLang, "xray", "func", "clock.purge_clock")
)

// TestRecord_ARecordInTheLaddersPickSettlesTheClaim (W-M1.41c): a record
// the request names whose evidence lies in the ladder's pick settles the
// claim: B4 applies to xray's timer, which the ladder picked by word
// coverage, so the claim is B4's on xray, banded in the named cell, and
// its line says the request names B4, which applies here.
func TestRecord_ARecordInTheLaddersPickSettlesTheClaim(t *testing.T) {
	prompt := "B4 again: fix the session purge timer."
	b, u := recordBrief(t, recordRepo(t), prompt, nil)
	r := b.Roles
	if want := (RecordClaim{IDs: []graph.ID{"les:B4"}, Units: 1}); r.Path != "xray" || r.Basis != BasisRecord || !sameRecord(r.Record, want) || r.RunnerUp != "" || r.Unnamed {
		t.Errorf("path %q basis %q record %+v runner-up %q unnamed %v, want xray on %+v", r.Path, r.Basis, r.Record, r.RunnerUp, r.Unnamed, want)
	}
	if r.cell() != BasisNamed || b.Lines[0].Band != BandStated || u.Basis != BasisRecord || u.Units != 1 {
		t.Errorf("cell %q band %q unit fact %+v, want the named cell, stated", r.cell(), b.Lines[0].Band, u)
	}
	if want := "unit 'xray' (the request names B4, which applies here) declares 'session_purge_timer'"; b.Lines[0].Text != want {
		t.Errorf("unit line\n got %q\nwant %q", b.Lines[0].Text, want)
	}
}

// TestRecord_AClaimOnOneUnitKeepsThePicksEvidenceItsRecordsApplyTo
// (W-M1.41c): a record claim on the ladder's pick keeps the evidence its
// records apply to, in the pick's order: a file and the symbols it
// declares (B4: the timer, not the clock; B5: the clock), all of the pick
// when a record applies to the unit as a whole (D6); a record that also
// applies in a unit the request bars claims the pick alone, and its line
// says so (B7 without gamma).
func TestRecord_AClaimOnOneUnitKeepsThePicksEvidenceItsRecordsApplyTo(t *testing.T) {
	g := heldRepo(t)
	for _, c := range []struct {
		prompt string
		rc     RecordClaim
		nodes  []graph.ID
		head   string
	}{
		{"B4 again: fix the session purge timer.", RecordClaim{IDs: []graph.ID{"les:B4"}, Units: 1}, []graph.ID{heldTimer, "file:xray/timer.py"},
			"unit 'xray' (the request names B4, which applies here) declares 'session_purge_timer';"},
		{"B5 again: fix the session purge timer.", RecordClaim{IDs: []graph.ID{"les:B5"}, Units: 1}, []graph.ID{heldClock},
			"unit 'xray' (the request names B5, which applies here) declares 'purge_clock';"},
		{"D6 again: fix the session purge timer.", RecordClaim{IDs: []graph.ID{"dec:D6"}, Units: 1}, []graph.ID{heldTimer, "file:xray/timer.py", heldClock},
			"unit 'xray' (the request names D6, which applies here) declares 'session_purge_timer';"},
		{"B7 again: fix the session purge timer, without touching gamma.", RecordClaim{IDs: []graph.ID{"les:B7"}, Units: 1, Barred: 1}, []graph.ID{heldClock},
			"unit 'xray' (the request names B7, which applies here and in 1 unit the request bars) declares 'purge_clock';"},
	} {
		b, _ := recordBrief(t, g, c.prompt, nil)
		r := b.Roles
		if r.Path != "xray" || r.Basis != BasisRecord || !sameRecord(r.Record, c.rc) || r.cell() != BasisNamed || b.Lines[0].Band != BandStated {
			t.Errorf("%q: path %q basis %q record %+v cell %q band %q, want xray on %+v in the named cell, stated", c.prompt, r.Path, r.Basis, r.Record, r.cell(), b.Lines[0].Band, c.rc)
		}
		if !slices.Equal(r.Picks[0].Nodes, c.nodes) {
			t.Errorf("%q: the claim keeps %v, want %v", c.prompt, r.Picks[0].Nodes, c.nodes)
		}
		if !strings.HasPrefix(b.Lines[0].Text, c.head) {
			t.Errorf("%q: unit line\n got %q\nwant prefix %q", c.prompt, b.Lines[0].Text, c.head)
		}
	}
}

// TestRecord_RecordsOnNoneOfThePickClaimOnTheirOwnEvidence (W-M1.41c): a
// record whose evidence lies in the ladder's unit but in none of its pick
// claims the unit on that evidence, as a record that outranks the pick
// does, since the line may not rest on the words alone: B8's idle loop
// file, where the ladder picked the timer and the clock; B9's symbol,
// which no file declares, where the ladder picked a symbol no file
// declares either.
func TestRecord_RecordsOnNoneOfThePickClaimOnTheirOwnEvidence(t *testing.T) {
	rev := graph.RevCommit(graphtest.Commit)
	orphan := synth(t, []synFile{{"xray", "xray/timer.py", []synSym{fn("session_purge_timer")}}}, nil, nil)
	for _, n := range []string{"tick.purge_timer_tick", "drift.idle_drift"} {
		orphan.AssertNode(graph.Node{ID: graph.SymID(synLang, "xray", "func", n), Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev,
			Attrs: map[string]string{"symkind": "func"}})
	}
	drift := graph.SymID(synLang, "xray", "func", "drift.idle_drift")
	record(t, orphan, "les:B9", "lesson")
	orphan.AssertEdge(graph.Edge{Src: "les:B9", Pred: graph.AppliesTo, Dst: drift, Tier: graph.Declared, Conf: 1, Rev: rev})
	for _, c := range []struct {
		g      *graph.Store
		prompt string
		id     graph.ID
		nodes  []graph.ID
		head   string
	}{
		{heldRepo(t), "B8 again: fix the session purge timer.", "les:B8", []graph.ID{"file:xray/idle.py"}, "unit 'xray' (the request names B8, which applies here)"},
		{orphan, "B9 again: fix the session purge timer.", "les:B9", []graph.ID{drift}, "unit 'xray' (the request names B9, which applies here)"},
	} {
		b, _ := recordBrief(t, c.g, c.prompt, nil)
		r := b.Roles
		if want := (RecordClaim{IDs: []graph.ID{c.id}, Units: 1}); r.Path != "xray" || r.Why != "named '"+string(c.id)+"' applies to xray" || !sameRecord(r.Record, want) || r.cell() != BasisNamed {
			t.Errorf("%q: path %q why %q record %+v cell %q, want xray on %s's own evidence in the named cell", c.prompt, r.Path, r.Why, r.Record, r.cell(), c.id)
		}
		if !slices.Equal(r.Picks[0].Nodes, c.nodes) {
			t.Errorf("%q: the claim keeps %v, want %v", c.prompt, r.Picks[0].Nodes, c.nodes)
		}
		if !strings.HasPrefix(b.Lines[0].Text, c.head) {
			t.Errorf("%q: unit line\n got %q\nwant prefix %q", c.prompt, b.Lines[0].Text, c.head)
		}
	}
}

// TestRecord_RecordsInSeveralUnitsOnTheLaddersPickAreBandedByItsReading
// (W-M1.41c): a record whose evidence lies in the ladder's pick and in a
// unit the request does not bar claims the pick as the ladder made it, on
// the record basis, banded in the ladder's cell: B7, on xray's clock and
// on gamma, claims xray, a candidate by word coverage with gamma its
// runner-up, and the claim keeps every node of the pick.
func TestRecord_RecordsInSeveralUnitsOnTheLaddersPickAreBandedByItsReading(t *testing.T) {
	prompt := "B7 again: fix the session purge timer."
	b, u := recordBrief(t, heldRepo(t), prompt, nil)
	r := b.Roles
	gamma := graph.UnitID(synLang, "gamma")
	if want := (RecordClaim{IDs: []graph.ID{"les:B7"}, Units: 2, First: BasisCoverage, RunnerUp: gamma}); r.Path != "xray" || r.Basis != BasisRecord || !sameRecord(r.Record, want) || r.RunnerUp != gamma {
		t.Errorf("path %q basis %q record %+v runner-up %q, want xray on %+v", r.Path, r.Basis, r.Record, r.RunnerUp, want)
	}
	if r.cell() != BasisCoverage || b.Lines[0].Band != BandCandidate || u.Units != 2 || u.First != BasisCoverage || u.RunnerUp != "gamma" {
		t.Errorf("cell %q band %q unit fact %+v, want the coverage cell, a candidate", r.cell(), b.Lines[0].Band, u)
	}
	if want := []graph.ID{heldTimer, "file:xray/timer.py", heldClock}; !slices.Equal(r.Picks[0].Nodes, want) {
		t.Errorf("the claim keeps %v, want the pick's %v", r.Picks[0].Nodes, want)
	}
	if head := "unit 'xray' (a candidate: the request names B7, which applies in 2 units; this one by word coverage; runner-up 'gamma') declares 'session_purge_timer';"; !strings.HasPrefix(b.Lines[0].Text, head) {
		t.Errorf("unit line\n got %q\nwant prefix %q", b.Lines[0].Text, head)
	}
}
