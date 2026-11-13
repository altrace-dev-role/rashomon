package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// sessionRecords is a session whose words overlap the prompts below
// (session, expiry, purge, timeout) and whose edges reach the specimen's
// own file and symbol nodes: the worst case for a corpus or a walk that
// fails to leave them out.
func sessionRecords(rev graph.Rev) *graph.SessionLog {
	return graph.OpenSessionLog("", "r", rev, true)
}

func withSession(sp *graphtest.Specimen) *graph.Store {
	rev := graph.RevCommit(graphtest.Commit)
	l := sessionRecords(rev)
	s := graph.NewStore()
	for _, n := range sp.Store.Nodes(rev) {
		s.AssertNode(n)
	}
	for _, e := range sp.Store.Edges(rev) {
		s.AssertEdge(e)
	}
	exit := 1
	recs := []graph.Record{
		l.Session("s1"),
		l.Prompt("s1", 1, "make session expiry respect the org-level timeout override purge_sessions"),
		l.Call("s1", "t1", graph.CallFields{Tool: "Bash", Command: "go test session expiry timeout purge_sessions", Exit: &exit, Output: "FAIL session purge", Runner: true}),
		l.Call("s1", "t2", graph.CallFields{Tool: "Read", Path: "jobs/purge_sessions.py"}),
		l.Call("s1", "t3", graph.CallFields{Tool: "Edit", Path: "jobs/purge_sessions.py", Diff: "-session\n+session expiry\n"}),
		l.Claim("s1", 1, "session expiry timeout done", "done", "unbacked"),
		l.Note("s1", 1, "UserPromptSubmit", "session expiry purge note"),
		l.Verdict("s1", 1, "go", "failed"),
		l.Edge(graph.SessionNodeID("s1"), graph.Asks, graph.PromptNodeID("s1", 1)),
		l.Edge(graph.SessionNodeID("s1"), graph.Acts, graph.CallNodeID("s1", "t1")),
		l.Edge(graph.CallNodeID("s1", "t2"), graph.ReadsFile, graph.FileID("jobs/purge_sessions.py")),
		l.Edge(graph.CallNodeID("s1", "t3"), graph.Edits, graph.FileID("jobs/purge_sessions.py")),
		l.Edge(graph.CallNodeID("s1", "t1"), graph.Runs, graph.VerdictNodeID("s1", 1)),
		l.Edge(graph.PromptNodeID("s1", 1), graph.Claims, graph.ClaimNodeID("s1", 1)),
		l.Edge(graph.ClaimNodeID("s1", 1), graph.EvidencedBy, graph.CallNodeID("s1", "t1")),
		l.Edge(graph.SessionNodeID("s1"), graph.Delivered, graph.NoteNodeID("s1", 1)),
		l.Edge(graph.NoteNodeID("s1", 1), graph.FollowedBy, graph.CallNodeID("s1", "t2")),
	}
	for _, r := range recs {
		if r.Node != nil {
			s.AssertNode(*r.Node)
		}
		if r.Edge != nil {
			s.AssertEdge(*r.Edge)
		}
	}
	return s
}

// stable drops the lines of an explanation that carry a wall-clock time.
func stable(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.Contains(l, "elapsed:") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// TestBrief_NonInterference_SessionRecords (W-M2.11, ruling F2): a brief
// built over a store that holds session records equals, byte for byte, the
// brief over the same store without them, for each scorer and each prompt.
func TestBrief_NonInterference_SessionRecords(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	with := withSession(sp)
	rev := graph.RevCommit(graphtest.Commit)
	if len(with.Nodes(rev)) <= len(sp.Store.Nodes(rev)) {
		t.Fatal("control: the session records did not enter the store")
	}
	prompts := []string{
		"make session expiry respect the org-level timeout override.",
		"block new logins for an org once it has too many live sessions.",
		"change the purge_sessions job so session expiry follows the timeout",
		"rename the session timeout setting",
	}
	for _, opt := range []Options{{}, {IDF: true}, {Lessons: true}} {
		for _, p := range prompts {
			a, b := Build(sp.Store, rev, p, opt), Build(with, rev, p, opt)
			if a.Text() != b.Text() || stable(a.Explain()) != stable(b.Explain()) {
				t.Errorf("opt %+v prompt %q: the brief moved with session records\nwithout:\n%s\nwith:\n%s", opt, p, stable(a.Explain()), stable(b.Explain()))
			}
		}
	}
}

// TestSessionPredicates_NotTraversed (ruling F1/F2): no session predicate
// is in the list a traversal follows, so a session edge is never walked.
func TestSessionPredicates_NotTraversed(t *testing.T) {
	walked := traversed()
	for _, p := range []graph.Pred{graph.Asks, graph.Acts, graph.ReadsFile, graph.Edits, graph.Runs, graph.Claims, graph.EvidencedBy, graph.Delivered, graph.FollowedBy} {
		if slices.Contains(walked, p) {
			t.Errorf("%q is traversed", p)
		}
	}
	if len(walked) == 0 {
		t.Error("traversed() is empty: an empty list means every predicate")
	}
}
