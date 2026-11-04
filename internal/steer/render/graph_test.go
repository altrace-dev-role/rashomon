package render

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

func sampleGraphLines() []slot.GraphLine {
	return []slot.GraphLine{
		slot.NewCodeLine(slot.CodeFact{Path: "../acme-ops/jobs/purge_sessions.py", Names: []slot.Ident{"run"},
			Writes: []slot.TableRef{{Table: "public.sessions", Cols: []slot.Ident{"created_at"}}}, Lessons: []slot.KnowledgeID{"les:INC-212"}}),
		slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeLesson, ID: "les:INC-212", Path: "../acme-ops/jobs/purge_sessions.py",
			Clause: "the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too",
			Source: "incidents/INC-212.md"}),
		slot.NewCodeLine(slot.CodeFact{Path: "services/auth/auth/sessions.py", Names: []slot.Ident{"create_session", "is_expired"},
			Consts: []slot.ConstFact{{Name: "SESSION_TTL_SECONDS", Value: 3600}}, Calls: []slot.Ident{"insert_session"}}),
	}
}

// TestGS2_GraphRowRendersUnderTheBriefingCheck (card GRAPH-SLOT1): the graph
// brief is a task-preflight row: a header with the revision and the two
// counts, then one sentence per line, each line a slot value, rendered
// under ValidateBriefing's rules (ASCII, no double quote, no banned word or
// second person outside a quote, six lines at most). An empty line slot is
// left out, so three lines render as a header and three sentences.
func TestGS2_GraphRowRendersUnderTheBriefingCheck(t *testing.T) {
	b := NewGraphBinding("C1", 4, 23, sampleGraphLines())
	m, err := New(pattern.TaskPreflight, b)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Logf("rendered:\n%s", m.Text())
	want := "rashomon record: the code this request touches, read from the repository graph at rev C1: 4 node(s) matched, 23 reached.\n" +
		"'../acme-ops/jobs/purge_sessions.py' declares 'run'; writes 'public.sessions' ('created_at'); INC-212 applies\n" +
		"lesson INC-212 applies to '../acme-ops/jobs/purge_sessions.py': 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'\n" +
		"'services/auth/auth/sessions.py' declares 'create_session', 'is_expired'; 'SESSION_TTL_SECONDS' is 3600; calls 'insert_session'"
	if m.Text() != want {
		t.Errorf("text\n got %q\nwant %q", m.Text(), want)
	}
	if m.Template() != "task-preflight-graph/v1" || m.Aside() != "" {
		t.Errorf("template %q aside %q", m.Template(), m.Aside())
	}
	if err := ValidateBriefing(m.Text()); err != nil {
		t.Errorf("the rendered brief fails the briefing check: %v", err)
	}
	if got := m.Slots(); len(got) != 8 || got[3].Name != "line1" || got[7].Name != "line5" {
		t.Errorf("slots = %d, want the eight of the binding", len(got))
	}
}

// TestGS2_ABriefWithNoLineIsRefused: a binding whose five lines are all
// empty has a header and nothing else, which the sparse template reads as
// no message (ErrNothing); the producer then has no finding.
func TestGS2_ABriefWithNoLineIsRefused(t *testing.T) {
	if _, err := New(pattern.TaskPreflight, NewGraphBinding("C1", 0, 0, nil)); err == nil {
		t.Error("a brief with no line rendered")
	}
}

// TestGS2_TheBriefNeverRaisesTheCap: five lines of near the longest shape
// each fit the 1,600-byte briefing cap only when their bytes do; a sixth
// line has no slot, and a brief over the cap is refused rather than cut,
// so the producer steps a line down.
func TestGS2_TheBriefNeverRaisesTheCap(t *testing.T) {
	long := slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeLesson, ID: "les:INC-212",
		Path:   slot.RelPath("../" + strings.Repeat("abcdefghij/", 9) + "a.py"),
		Clause: slot.Approved(strings.Repeat("word ", 31) + "end"), Source: slot.Path(strings.Repeat("docs/", 20) + "a.md")})
	lines := []slot.GraphLine{long, long, long, long, long, long}
	b := NewGraphBinding("C1", 1, 1, lines)
	if _, err := New(pattern.TaskPreflight, b); err == nil {
		t.Error("five lines over the cap rendered")
	}
	m, err := New(pattern.TaskPreflight, NewGraphBinding("C1", 1, 1, lines[:3]))
	if err != nil {
		t.Fatalf("three long lines: %v", err)
	}
	if n := strings.Count(m.Text(), "\n"); n != 3 {
		t.Errorf("three lines rendered as %d line feeds", n)
	}
}
