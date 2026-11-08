package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestGS6_AKnowledgeNodeNeverSeeds (card GRAPH-SLOT1): a prompt made of a
// lesson's own words seeds no lesson and no decision; knowledge reaches
// the brief only by applying to code the words selected. This is the B'
// confound's fix: the earlier scorer let a lesson's text vote for itself.
func TestGS6_AKnowledgeNodeNeverSeeds(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	for _, opt := range []Options{{}, {IDF: true}} {
		seeds, rows, _ := Scope(sp.Store, graph.RevCommit(graphtest.Commit), "nightly purge raced the TTL lifetime predicate", opt)
		for _, r := range rows {
			if IsKnowledge(r.Kind) {
				t.Errorf("scorer idf=%v scored %s (%s)", opt.IDF, r.ID, r.Kind)
			}
		}
		for _, s := range seeds {
			if n, _ := sp.Store.Node(s.ID, graph.RevCommit(graphtest.Commit)); IsKnowledge(n.Kind) {
				t.Errorf("scorer idf=%v seeded %s", opt.IDF, s.ID)
			}
		}
	}
}

// TestGS6_EachLineIsASlotRenderedByTheGraphTemplate: Build binds each
// selected line's facts into a graph-line slot and renders the brief
// through the task-preflight graph template, paths in the form Rel gives
// (the hook's cwd), the lesson's path the file its record declares, and
// the record form holds digests and counts in the lesson's and paths'
// place. Explain names each line's sentence.
func TestGS6_EachLineIsASlotRenderedByTheGraphTemplate(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	rel := func(p string) string { return "../../" + p }
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true, Rel: rel, bands: stateAll})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	if b.Empty() || b.Kept != len(b.Lines) || b.Kept > MaxLines {
		t.Fatalf("kept %d of %d lines", b.Kept, len(b.Lines))
	}
	if b.Message.Template() != "task-preflight-graph/v1" {
		t.Errorf("template %q", b.Message.Template())
	}
	want := "lesson INC-212 applies to '../../jobs/purge_sessions.py': failure 'INC-212 session purge raced the TTL'; rule 'the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too'; source 'incidents/INC-212.md'"
	if !strings.Contains(b.Text(), want) {
		t.Errorf("the lesson line is not\n%s\nin\n%s", want, b.Text())
	}
	if !strings.HasPrefix(b.Text(), "rashomon record: the code this request touches, read from the repository graph at rev C1: ") {
		t.Errorf("header: %q", strings.SplitN(b.Text(), "\n", 2)[0])
	}
	for i, l := range b.Lines {
		if l.Text == "" || !strings.Contains(b.Text(), l.Text) {
			t.Errorf("line %d text %q is not in the brief", i+1, l.Text)
		}
	}
	rec, err := b.Record(func(string) string { return strings.Repeat("f", 64) })
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	t.Logf("record form:\n%s", rec.Text())
	if strings.Contains(rec.Text(), "purge_sessions") || !strings.Contains(rec.Text(), "INC-212 applies to a file on record") {
		t.Errorf("record form:\n%s", rec.Text())
	}
	if !strings.Contains(b.Explain(), "render (") || !strings.Contains(b.Explain(), "task-preflight-graph/v1") {
		t.Errorf("Explain lacks the render section:\n%s", b.Explain())
	}
}

// TestGS6_ALineTheSlotRefusesIsDroppedWithADecision: a file whose path a
// check refuses (a secret-shaped segment) is left out of the brief, and
// the decision names the refusal; the other lines render.
func TestGS6_ALineTheSlotRefusesIsDroppedWithADecision(t *testing.T) {
	sp := graphtest.New(t)
	rev := graph.RevCommit(graphtest.Commit)
	bad := graph.FileID("jobs/AKIAABCDEFGHIJKLMNOP.py")
	sp.Store.AssertNode(graph.Node{ID: bad, Kind: "file", Tier: graph.Declared, Conf: 1.0, Rev: rev})
	sym := graph.SymID("py", "jobs", "func", "jobs.AKIAABCDEFGHIJKLMNOP.purge_sessions")
	sp.Store.AssertNode(graph.Node{ID: sym, Kind: "sym", Tier: graph.Declared, Conf: 1.0, Rev: rev, Attrs: map[string]string{"symkind": "func"}})
	sp.Store.AssertEdge(graph.Edge{Src: bad, Pred: graph.Declares, Dst: sym, Tier: graph.Declared, Conf: 1.0, Rev: rev})
	sp.Store.AssertEdge(graph.Edge{Src: sym, Pred: graph.Writes, Dst: graph.TableID("public.sessions"), Tier: graph.Declared, Conf: 1.0, Rev: rev})
	b := Build(sp.Store, rev, "purge sessions", Options{})
	t.Logf("\n%s", b.Explain())
	refused := false
	for _, d := range b.Decisions {
		refused = refused || d.ID == bad && strings.HasPrefix(d.Why, "refused: ")
	}
	if !refused {
		t.Errorf("no refusal decision for %s", bad)
	}
	if strings.Contains(b.Text(), "AKIA") {
		t.Errorf("the refused path reached the brief:\n%s", b.Text())
	}
	for _, l := range b.Lines {
		if l.Key == bad {
			t.Error("the refused line stayed among the lines")
		}
	}
}

// TestGS7_LessonsOnlyMatchesKnowledgeByItsWordsAlone (card GRAPH-ABL1): the
// ablation arm matches the prompt's words against the knowledge records
// alone and renders them as knowledge lines through the same slot and
// template; it traverses nothing, so no file, table or symbol line exists,
// its neighborhood is its matches, and a prompt whose words hit no record
// gives no brief.
func TestGS7_LessonsOnlyMatchesKnowledgeByItsWordsAlone(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	g, rev := sp.Store, graph.RevCommit(graphtest.Commit)
	opt := Options{IDF: true, Lessons: true, Rel: func(p string) string { return "../../" + p }}
	b := Build(g, rev, "make session expiry respect the org-level timeout override.", opt)
	if b.Scorer != "lessons" || b.Empty() {
		t.Fatalf("scorer %q empty %v", b.Scorer, b.Empty())
	}
	for _, r := range b.Rows {
		if !IsKnowledge(r.Kind) {
			t.Errorf("a %s node scored: %s", r.Kind, r.ID)
		}
	}
	if b.Neighborhood != len(b.Seeds) || len(b.Candidates) != 0 {
		t.Errorf("neighborhood %d seeds %d candidates %d: the arm traversed", b.Neighborhood, len(b.Seeds), len(b.Candidates))
	}
	for _, l := range b.Lines {
		if _, ok := l.Slot.Knowledge(); !ok {
			t.Errorf("a line that is not a knowledge line: %s", l.Key)
		}
	}
	if !strings.Contains(b.Text(), "lesson INC-212 applies to '../../jobs/purge_sessions.py'") {
		t.Errorf("the lesson line lacks its declared file:\n%s", b.Text())
	}
	if full := Build(g, rev, "make session expiry respect the org-level timeout override.", Options{IDF: true, Rel: opt.Rel}); full.Scorer != "idf" || len(full.Candidates) == 0 {
		t.Errorf("the full arm changed: scorer %q candidates %d", full.Scorer, len(full.Candidates))
	}
	if none := Build(g, rev, "rotate the signing keys.", opt); !none.Empty() {
		t.Errorf("a prompt hitting no record gave a brief:\n%s", none.Text())
	}
}
