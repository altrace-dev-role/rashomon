package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead (card GRAPH-REACH1):
// from services/auth, the purge job in jobs/ lies outside the working
// directory, so it gains ReachBonus among the candidates; from the index
// root nothing is outside and nothing gains.
// On the role lines (W-M1.12) the clause is the consumer order: the unit
// has two consumers outside it, the purge job and the test file, so the
// first consumer line from jobs/ names the test file the agent cannot
// open by name, from tests/ the purge job, and from the root, where both
// are in reach, the one the prompt's words reach.
// No lesson or decision is in the store: the first clause of the product
// statement tested alone.
// No unit rule applies here: the shipped one (W-M1.40) withholds a unit
// the request does not name.
func TestGR1_OutOfReachCodeRanksAboveWhatTheAgentCanRead(t *testing.T) {
	sp := graphtest.New(t)
	g, rev := sp.Store, graph.RevCommit(graphtest.Commit)
	prompt := "make session expiry respect the org-level timeout override."
	from := func(dir string) func(string) string {
		return func(p string) string {
			r, err := filepath.Rel(dir, p)
			if err != nil {
				t.Fatal(err)
			}
			return filepath.ToSlash(r)
		}
	}
	cwd := Build(g, rev, prompt, Options{IDF: true, Rel: from("services/auth"), unitRule: noUnitRule})
	root := Build(g, rev, prompt, Options{IDF: true, unitRule: noUnitRule})
	t.Logf("from services/auth:\n%s\nbrief:\n%s", cwd.Explain(), cwd.Text())
	t.Logf("from the root:\n%s\nbrief:\n%s", root.Explain(), root.Text())
	const purge, sessions = graph.ID("file:jobs/purge_sessions.py"), graph.ID("file:services/auth/auth/sessions.py")
	find := func(b Brief, id graph.ID) Candidate {
		for _, c := range b.Candidates {
			if c.ID == id {
				return c
			}
		}
		t.Fatalf("%s is not a candidate", id)
		return Candidate{}
	}
	for _, id := range []graph.ID{purge} {
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
	// The first consumer line names what the agent cannot open by name.
	consumerLine := func(b Brief) string {
		for _, l := range b.Lines[:b.Kept] {
			if l.Role == RoleConsumer {
				return l.Text
			}
		}
		return ""
	}
	cases := []struct{ dir, want string }{
		{"services/auth", "consumer '../../jobs/purge_sessions.py'"},
		{"jobs", "consumer '../tests/unit/test_sessions.py'"},
		{"tests", "consumer '../jobs/purge_sessions.py'"},
		{"", "consumer 'jobs/purge_sessions.py'"},
	}
	for _, tc := range cases {
		opt := Options{IDF: true, unitRule: noUnitRule}
		if tc.dir != "" {
			opt.Rel = from(tc.dir)
		}
		b := Build(g, rev, prompt, opt)
		if got := consumerLine(b); !strings.HasPrefix(got, tc.want) {
			t.Errorf("from %q the first consumer line is %q, want prefix %q\n%s", tc.dir, got, tc.want, b.Text())
		}
	}
}

// TestGR1_OutsideJudgesEachKindByItsOwnRule: Outside over a store declared
// by hand, from the working directory services/auth.
// A file is outside by its path, a symbol by the file that declares it, a
// table by any symbol that writes or reads it. The jobs file and the
// symbol it declares are outside; the auth file and its symbol are not.
// The sessions table, written by the jobs symbol and read by the auth
// symbol, is outside; the audit table, written and read by the auth
// symbol alone, is not. Each branch has a case of each answer. No reader
// writes a table since pylite/3, so the tables here are declared by hand.
func TestGR1_OutsideJudgesEachKindByItsOwnRule(t *testing.T) {
	f := newFx()
	jobs := f.file("jobs", "jobs/purge.py", fxC1)
	auth := f.file("services/auth", "services/auth/auth/sessions.py", fxC1)
	purge := f.sym(jobs, "jobs", "func", "purge.run", fxC1, nil)
	expired := f.sym(auth, "services/auth", "func", "auth.sessions.is_expired", fxC1, nil)
	sessions, audit := graph.TableID("sessions"), graph.TableID("audit")
	for _, id := range []graph.ID{sessions, audit} {
		f.s.AssertNode(graph.Node{ID: id, Kind: "table", Rev: fxC1})
	}
	f.edge(purge, graph.Writes, sessions, fxC1, nil)
	f.edge(expired, graph.Reads, sessions, fxC1, nil)
	f.edge(expired, graph.Writes, audit, fxC1, nil)
	f.edge(expired, graph.Reads, audit, fxC1, nil)
	outside := Outside(f.s, fxC1, func(p string) string {
		r, err := filepath.Rel("services/auth", p)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(r)
	})
	for _, tc := range []struct {
		id   graph.ID
		want bool
	}{
		{jobs, true},
		{auth, false},
		{purge, true},
		{expired, false},
		{sessions, true},
		{audit, false},
	} {
		if got := outside(tc.id); got != tc.want {
			t.Errorf("Outside(%s) = %v, want %v", tc.id, got, tc.want)
		}
	}
}
