package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead (card GRAPH-REACH1):
// from services/auth, the purge job in jobs/ and the table it writes lie
// outside the working directory, so they gain ReachBonus and the purge
// file takes a line above the files the agent can open by name; from the
// index root nothing is outside, nothing gains, and the brief is the one
// built before the term existed. No lesson or decision is in the store:
// the first clause of the product statement tested alone.
func TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead(t *testing.T) {
	sp := graphtest.New(t)
	g, rev := sp.Store, graph.RevCommit(graphtest.Commit)
	prompt := "make session expiry respect the org-level timeout override."
	fromAuth := func(p string) string {
		r, err := filepath.Rel("services/auth", p)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(r)
	}
	cwd := Build(g, rev, prompt, Options{IDF: true, Rel: fromAuth})
	root := Build(g, rev, prompt, Options{IDF: true})
	t.Logf("from services/auth:\n%s\nbrief:\n%s", cwd.Explain(), cwd.Text())
	t.Logf("from the root:\n%s\nbrief:\n%s", root.Explain(), root.Text())
	const purge, table, sessions = graph.ID("file:jobs/purge_sessions.py"), graph.ID("tbl:public.sessions"), graph.ID("file:services/auth/auth/sessions.py")
	find := func(b Brief, id graph.ID) Candidate {
		for _, c := range b.Candidates {
			if c.ID == id {
				return c
			}
		}
		t.Fatalf("%s is not a candidate", id)
		return Candidate{}
	}
	for _, id := range []graph.ID{purge, table} {
		if c := find(cwd, id); !c.Reach || c.Score != c.Base+ReachBonus || c.Score <= c.Base {
			t.Errorf("from services/auth %s = %+v, want Reach and base+%.2f", id, c, ReachBonus)
		}
	}
	if c := find(cwd, sessions); c.Reach || c.Score != c.Base {
		t.Errorf("from services/auth the cwd's own file %s = %+v, want in reach at its base", sessions, c)
	}
	for _, c := range root.Candidates {
		if c.Reach || c.Score != c.Base+KnowledgeBonus*float64(min(len(c.Knowledge), 1)) {
			t.Errorf("from the root %s = %+v, want nothing out of reach", c.ID, c)
		}
	}
	// The purge file's line is in the brief and above every file line the
	// agent can open by name; from the root it is cut.
	at := func(b Brief, id graph.ID) int {
		for i, l := range b.Lines[:b.Kept] {
			if l.Key == id {
				return i
			}
		}
		return -1
	}
	if i := at(cwd, purge); i < 0 || !strings.HasPrefix(cwd.Lines[i].Text, "'../../jobs/purge_sessions.py'") {
		t.Fatalf("from services/auth the purge file is line %d of\n%s", i+1, cwd.Text())
	}
	for i, l := range cwd.Lines[:at(cwd, purge)] {
		if strings.HasPrefix(string(l.Key), "file:") && !strings.HasPrefix(l.Text, "'../") {
			t.Errorf("line %d %q is in reach yet above the purge file", i+1, l.Text)
		}
	}
	if i := at(root, purge); i >= 0 {
		t.Errorf("from the root the purge file is line %d; the as-built rank cut it", i+1)
	}
}
