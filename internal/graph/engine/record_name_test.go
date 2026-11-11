package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The record claim by the unit's own name (W-M1.49): records that apply
// in several units, none of which the ladder ranks, claim the one unit
// among them the request names by its own name. nameRepo's units are
// harness, steer, gateout and session, each declaring a name no request
// here holds; D7 applies in the first three. nameRequest is classes3.tsv
// KI18 with D7 for D181, the product named as subject and the last
// sentence as last: its frame names no target, so the ladder reads the
// request's things by the units' names, and they name session ("the very
// first prompt of a session") beside any unit the subject names, two
// units, so the ladder ranks none, as on KI18.
func nameRepo(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"harness", "harness/kiln.py", []synSym{fn("kiln_spool")}},
		{"steer", "steer/quill.py", []synSym{fn("quill_lattice")}},
		{"gateout", "gateout/fjord.py", []synSym{fn("fjord_mantle")}},
		{"session", "session/oak.py", []synSym{fn("oak_ember")}},
	}, nil, nil)
	record(t, g, "dec:D7", "decision", "harness/kiln.py", "steer/quill.py", "gateout/fjord.py")
	return g
}

const ki18Last = "Don't touch anything about the positive control the other measurement work already depends on."

func nameRequest(subject, last string) string {
	return "D7 explains why a non-interactive, scripted run of the tool can't be used to measure most of what " + subject +
		" normally shows: that surface's in-band mirror reads as unverified, so almost every post-event note gets suppressed" +
		" and only the very first prompt of a session is exempt. I want to start closing that gap by seeing if we can earn a" +
		" verified reading for that surface instead of leaving it permanently unverified. Tell me where the capability table" +
		" would need to change to let a surface move out of unverified, and whether the suppression logic downstream would" +
		" need to change to notice. " + last
}

// TestRecord_ARecordInSeveralUnitsClaimsTheOneTheRequestNamesByItsName:
// "AutoSteer" holds steer, so D7 claims steer on the record basis, its
// three units counted and steer ranked first among them by its name, with
// no runner-up; the claim is banded in the unit-name cell, a candidate,
// and line 1 says so. A unit the request bars is none of the records'
// units (recordUnits) whatever spelling names it: barring gateout, which
// "the gate output" also holds, leaves steer the one unit named, on two
// units counted and one barred.
func TestRecord_ARecordInSeveralUnitsClaimsTheOneTheRequestNamesByItsName(t *testing.T) {
	g := nameRepo(t)
	for _, c := range []struct {
		prompt string
		want   RecordClaim
		head   string
	}{
		{nameRequest("AutoSteer", ki18Last), RecordClaim{IDs: []graph.ID{"dec:D7"}, Units: 3, First: BasisUnitName},
			"unit 'steer' (a candidate: the request names D7, which applies in 3 units; this one by its name)"},
		{nameRequest("AutoSteer in the gate output", "Don't touch gateout."),
			RecordClaim{IDs: []graph.ID{"dec:D7"}, Units: 2, Barred: 1, First: BasisUnitName}, "unit 'steer' (a candidate: "},
	} {
		b, u := recordBrief(t, g, c.prompt, nil)
		r := b.Roles
		if r.Path != "steer" || r.Basis != BasisRecord || !sameRecord(r.Record, c.want) || r.RunnerUp != "" || r.Unnamed {
			t.Errorf("%q: path %q basis %q record %+v runner-up %q unnamed %v, want steer on %+v", c.prompt, r.Path, r.Basis, r.Record, r.RunnerUp, r.Unnamed, c.want)
		}
		if r.cell() != BasisUnitName || b.Lines[0].Band != BandCandidate {
			t.Errorf("%q: cell %q band %q, want the unit-name cell, a candidate", c.prompt, r.cell(), b.Lines[0].Band)
		}
		if u.Basis != BasisRecord || u.Units != c.want.Units || u.First != BasisUnitName || u.RunnerUp != "" {
			t.Errorf("%q: unit fact %+v", c.prompt, u)
		}
		if !strings.HasPrefix(b.Lines[0].Text, c.head) {
			t.Errorf("%q: unit line\n got %q\nwant prefix %q", c.prompt, b.Lines[0].Text, c.head)
		}
	}
}

// TestRecord_ARecordInSeveralUnitsClaimsNoUnitTheRequestDoesNotNameAlone:
// the records claim by name only the one unit of theirs the request
// names: naming two of their units, none, or only the one it bars makes
// no record claim, whatever the ladder claims (session by its own name,
// when it is the one unit named).
func TestRecord_ARecordInSeveralUnitsClaimsNoUnitTheRequestDoesNotNameAlone(t *testing.T) {
	g := nameRepo(t)
	for _, prompt := range []string{
		nameRequest("AutoSteer and the harness", ki18Last),
		nameRequest("the product", ki18Last),
		nameRequest("AutoSteer", "Don't touch steer."),
	} {
		b := Build(g, graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true})
		t.Logf("%q:\n%s\nbrief:\n%s", prompt, b.Explain(), b.Text())
		if r := b.Roles; r.Basis == BasisRecord || r.Record.Units != 0 || slices.Contains([]string{"harness", "steer", "gateout"}, r.Path) {
			t.Errorf("%q: path %q basis %q record %+v, want no claim by the records", prompt, r.Path, r.Basis, r.Record)
		}
	}
}
