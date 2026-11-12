package render

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// sampleGraphText is sampleGraphLines as the graph rows render them, after
// the head.
const sampleGraphText = "'../acme-ops/jobs/purge_sessions.py' declares 'run'; writes 'public.sessions' ('created_at'); INC-212 applies\n" +
	"lesson INC-212 applies to '../acme-ops/jobs/purge_sessions.py': rule 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'\n" +
	"'services/auth/auth/sessions.py' declares 'create_session', 'is_expired'; 'SESSION_TTL_SECONDS' is 3600; calls 'insert_session'"

// TestUnitBriefRow_NamesTheUnitNotTheRequest (W-M2.3 hook half, ruling
// 04:58 B): the unit brief's row says the graph's lines under a head that
// names the unit, the root unit as '.', and the revision, with no request
// and no node counts; the lines are the graph row's, byte for byte, under
// the briefing check, and the row has no aside. Break: give the row the
// graph head, and the text is the request's.
func TestUnitBriefRow_NamesTheUnitNotTheRequest(t *testing.T) {
	for _, c := range []struct {
		unit slot.RelPath
		head string
	}{
		{"internal/store", "rashomon record: an edit in unit 'internal/store', noted once a session; the repository graph at rev C1 records for it:\n"},
		{".", "rashomon record: an edit in unit '.', noted once a session; the repository graph at rev C1 records for it:\n"},
	} {
		m, err := New(pattern.TaskPreflight, NewUnitBriefBinding(c.unit, "C1", sampleGraphLines()))
		if err != nil {
			t.Fatalf("%s: New: %v", c.unit, err)
		}
		if want := c.head + sampleGraphText; m.Text() != want {
			t.Errorf("%s: text\n got %q\nwant %q", c.unit, m.Text(), want)
		}
		if m.Template() != "task-preflight-unitbrief/v1" || m.Aside() != "" {
			t.Errorf("%s: template %q aside %q", c.unit, m.Template(), m.Aside())
		}
		if err := ValidateBriefing(m.Text()); err != nil {
			t.Errorf("%s: the rendered brief fails the briefing check: %v", c.unit, err)
		}
		if got := m.Slots(); len(got) != 7 || got[0].Name != "unit" || got[0].Value != c.unit {
			t.Errorf("%s: slots %v, want the seven of the binding, the unit first", c.unit, got)
		}
	}
}

// TestUnitBriefRow_RecordHoldsNoUnit: the record row is the same head with
// the unit as one on record and no unit slot, over the lines' record form,
// so at record scope v4 the steer line holds no path of the unit's. A unit
// path the slot refuses renders no row, and neither does a row with no
// line (sparse, as the graph row). Witness: the plain row over the same
// lines names the unit.
func TestUnitBriefRow_RecordHoldsNoUnit(t *testing.T) {
	digest := func(string) string { return strings.Repeat("ab", 32) }
	var rec []slot.GraphLine
	for _, l := range sampleGraphLines() {
		rec = append(rec, l.Record(digest))
	}
	m, err := New(pattern.TaskPreflight, NewUnitBriefRecordBinding("C1", rec))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	head := "rashomon record: an edit in a unit on record, noted once a session; the repository graph at rev C1 records for it:\n"
	if !strings.HasPrefix(m.Text(), head) || strings.Count(m.Text(), "\n") != 3 {
		t.Errorf("record text %q, want the record head and three lines", m.Text())
	}
	if m.Template() != "task-preflight-unitbrief-record/v1" {
		t.Errorf("template %q", m.Template())
	}
	for _, s := range m.Slots() {
		if s.Name == "unit" {
			t.Errorf("the record row holds a unit slot %v", s.Value)
		}
	}
	plain, err := New(pattern.TaskPreflight, NewUnitBriefBinding("internal/store", "C1", rec))
	if err != nil || !strings.Contains(plain.Text(), "'internal/store'") {
		t.Fatalf("witness: the plain row %q (%v) does not name the unit", plain.Text(), err)
	}
	if strings.Contains(m.Text(), "internal/store") {
		t.Errorf("the record row names the unit: %q", m.Text())
	}
	if _, err := New(pattern.TaskPreflight, NewUnitBriefBinding("/abs/store", "C1", sampleGraphLines())); err == nil {
		t.Error("a unit path the slot refuses rendered")
	}
	if _, err := New(pattern.TaskPreflight, NewUnitBriefBinding("internal/store", "C1", nil)); err == nil {
		t.Error("a unit brief with no line rendered")
	}
}
