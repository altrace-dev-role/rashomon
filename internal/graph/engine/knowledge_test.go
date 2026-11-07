package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestKnowledge_AConsumersRecordNeverExpandsOnAUnitWithNone (W-M1.23,
// AC-M1.5): the knowledge line is the unit's own record. A unit with no
// record of its own gets no knowledge line, whatever its consumers carry:
// here the brief module consumes the store's loader and carries a lesson
// on itself; the brief's record is the consumer line's to show ("B1
// applies"), never a knowledge line for the store. On the W-M1.19 set 25
// of the 160 rows carried a knowledge line for a unit with no record and
// 36 a neighbour's record.
func TestKnowledge_AConsumersRecordNeverExpandsOnAUnitWithNone(t *testing.T) {
	g := synth(t, []synFile{
		{"store", "store/loader.py", []synSym{fn("load_rows")}},
		{"brief", "brief/brief.py", []synSym{fn("brief")}},
	}, [][2]string{{"brief/brief.py#brief", "store/loader.py#load_rows"}}, nil)
	record(t, g, "les:B1", "lesson", "brief/brief.py")
	r := roles(t, g, "fix load_rows in the store.")
	if r.Unit != graph.UnitID(synLang, "store") || len(r.Consumers) != 1 || r.Consumers[0].File != graph.FileID("brief/brief.py") {
		t.Fatalf("roles %+v", r)
	}
	if len(r.Knowledge) != 0 {
		t.Errorf("knowledge %v, want none: the store has no record of its own", r.Knowledge)
	}
}

// TestKnowledge_TheUnitsOwnRecordIsTheLine: on Task A the unit's own
// record is the lesson its db module reaches through the table it writes
// (public.sessions), found through that file; the decision on the reuse
// (orgs.settings) is the reuse line's to show and no knowledge line.
func TestKnowledge_TheUnitsOwnRecordIsTheLine(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	r := b.Roles
	if len(r.Knowledge) != 1 || r.Knowledge[0] != "les:INC-212" {
		t.Errorf("knowledge %v, want the unit's own lesson alone", r.Knowledge)
	}
	if via := r.KnowledgeVia["les:INC-212"]; via != graph.FileID("services/auth/auth/db.py") {
		t.Errorf("INC-212 found through %s, want the unit's db module", via)
	}
	if !strings.Contains(b.Text(), "reuse 'services/orgs/orgs/settings.py' (named to use) declares 'get_org_settings'; imports 'orgs.models'; ADR-0007 applies") {
		t.Errorf("the reuse line does not carry its record:\n%s", b.Text())
	}
	if strings.Contains(b.Text(), "decision ADR-0007") {
		t.Errorf("the reuse's record became a knowledge line:\n%s", b.Text())
	}
}

// TestKnowledge_ALessonLineNamesTheFailureAndTheRule (W-M1.25): the
// knowledge line states the record's standing by kind, as fact: Task A's
// lesson names the failure it was drawn from (the record's title) and the
// rule (its clause), through the renderer's statement-of-fact check
// (LUI-11), which refused the imperative form on this very brief.
func TestKnowledge_ALessonLineNamesTheFailureAndTheRule(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	want := "lesson INC-212 applies to 'jobs/purge_sessions.py': failure 'INC-212 session purge raced the TTL'; rule 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'"
	if !strings.Contains(b.Text(), want) {
		t.Errorf("brief lacks the lesson's polarity line\nwant %q\n%s", want, b.Text())
	}
}
