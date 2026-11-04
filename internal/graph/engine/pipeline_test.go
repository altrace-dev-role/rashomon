package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The specimen's knowledge: one incident on the sessions table, one
// decision on how auth reads org settings.
func withKnowledge(sp *graphtest.Specimen) {
	rev := graph.RevCommit(graphtest.Commit)
	les := graph.ID("les:INC-212")
	sp.Store.AssertNode(graph.Node{ID: les, Kind: "lesson", Tier: graph.Declared, Conf: 1.0, Rev: rev, Attrs: map[string]string{
		"title":  "INC-212 session purge raced the TTL",
		"text":   "INC-212: the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS; any change to session lifetime must change the purge predicate too, or live sessions are deleted (2025-08).",
		"clause": "the nightly purge deletes public.sessions by created_at + SESSION_TTL_SECONDS, so a session-lifetime change changes the purge predicate too",
		"source": "incidents/INC-212.md"}})
	sp.Store.AssertEdge(graph.Edge{Src: les, Pred: graph.AppliesTo, Dst: graph.TableID("public.sessions"), Tier: graph.Declared, Conf: 1.0, Rev: rev})
	sp.Store.AssertEdge(graph.Edge{Src: les, Pred: graph.AppliesTo, Dst: graph.FileID("jobs/purge_sessions.py"), Tier: graph.Declared, Conf: 1.0, Rev: rev})
	dec := graph.ID("dec:ADR-0007")
	sp.Store.AssertNode(graph.Node{ID: dec, Kind: "decision", Tier: graph.Declared, Conf: 1.0, Rev: rev, Attrs: map[string]string{
		"title":  "ADR-0007 auth reads org settings through orgs.settings",
		"text":   "ADR-0007: auth reads org values only through orgs.settings.get_org_settings(org_id); it never imports orgs.models.",
		"clause": "auth reads org values only through orgs.settings.get_org_settings(org_id) and never imports orgs.models",
		"source": "docs/adr/0007.md"}})
	sp.Store.AssertEdge(graph.Edge{Src: dec, Pred: graph.AppliesTo, Dst: graphtest.Sym("services/orgs", "orgs.settings.get_org_settings"), Tier: graph.Declared, Conf: 1.0, Rev: rev})
}

// TestPipeline_TaskA: the brief for Task A names the purge job and carries
// INC-212 before the agent reads a file.
func TestPipeline_TaskA(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	for _, opt := range []Options{{}, {IDF: true}} {
		b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", opt)
		t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
		if !strings.Contains(b.Text(), "jobs/purge_sessions.py") {
			t.Fatalf("brief does not name the purge job:\n%s", b.Text())
		}
		if !strings.Contains(b.Text(), "INC-212") {
			t.Fatalf("brief does not carry INC-212:\n%s", b.Text())
		}
	}
}

// TestPipeline_TaskBPrime: Task B' has no seeded knowledge pointing at
// active_sessions. The test records what the pipeline does with it; the
// result is a finding, not a requirement.
func TestPipeline_TaskBPrime(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	for _, opt := range []Options{{}, {IDF: true}} {
		b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "block new logins for an org once it has too many live sessions.", opt)
		t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
		t.Logf("scorer %s: active_sessions selected: %v", b.Scorer, strings.Contains(b.Text(), "reports/daily.py"))
	}
}
