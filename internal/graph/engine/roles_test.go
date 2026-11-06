package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestRoles_TaskANamesTheFourRoles (W-M1.12, AC-M1.1's shape): for Task A
// the brief's first five lines are the unit that owns the task (the
// target is "session expiry", what "respect" opens is the use phrase), the
// file it enters by, the abstraction to reuse (the org settings accessor
// names a word of the use phrase; "session" and "expiry" named the target
// and are spent, so insert_session rests on a doc mention alone and
// ranks after it; the daily report's doc mention of org_id outside the
// unit is no reuse), the consumer outside the unit, and the knowledge
// that applies, the lesson on the consumer before the decision on the
// reuse; the decision is found and cut at the budget with a decision
// saying so.
func TestRoles_TaskANamesTheFourRoles(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	r := b.Roles
	auth := graph.UnitID("py", "services/auth")
	if r.Unit != auth || r.File != graph.FileID("services/auth/auth/sessions.py") || r.Why != ClassFull {
		t.Fatalf("roles %+v", r)
	}
	if len(r.Reuse) == 0 || r.Reuse[0].File != graph.FileID("services/orgs/orgs/settings.py") {
		t.Errorf("reuse %+v", r.Reuse)
	}
	for _, rf := range r.Reuse[1:] {
		if !strings.HasPrefix(string(rf.File), "file:services/auth/") {
			t.Errorf("reuse %s outside the unit on a doc mention", rf.File)
		}
	}
	if len(r.Consumers) == 0 || r.Consumers[0].File != graph.FileID("jobs/purge_sessions.py") {
		t.Errorf("consumers %+v", r.Consumers)
	}
	if !containsID(r.UnitConsumers, graph.FileID("tests/unit/test_sessions.py")) {
		t.Errorf("unit consumers %v lack the test", r.UnitConsumers)
	}
	if len(r.Knowledge) != 2 || r.Knowledge[0] != "les:INC-212" || r.Knowledge[1] != "dec:ADR-0007" {
		t.Errorf("knowledge %v", r.Knowledge)
	}
	roles := []string{RoleUnit, RoleTarget, RoleReuse, RoleConsumer, RoleKnowledge}
	if b.Kept != 5 {
		t.Fatalf("kept %d of %d", b.Kept, len(b.Lines))
	}
	for i, want := range roles {
		if b.Lines[i].Role != want {
			t.Errorf("line %d role %q, want %q", i+1, b.Lines[i].Role, want)
		}
	}
	lines := strings.Split(b.Text(), "\n")[1:]
	wants := []string{
		"unit 'services/auth' declares 'is_expired', 'create_session', 'SESSION_TTL_SECONDS'; is consumed by 'jobs/purge_sessions.py', 'tests/unit/test_sessions.py' and 1 more",
		"'services/auth/auth/sessions.py' declares",
		"reuse 'services/orgs/orgs/settings.py' declares 'get_org_settings'",
		"consumer 'jobs/purge_sessions.py'",
		"lesson INC-212 applies to 'jobs/purge_sessions.py'",
	}
	for i, want := range wants {
		if i >= len(lines) || !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d\n got %q\nwant prefix %q", i+1, lines[i], want)
		}
	}
	cut := false
	for _, d := range b.Decisions {
		cut = cut || d.ID == "dec:ADR-0007" && d.Why == "cut: budget (role knowledge)"
	}
	if !cut {
		t.Errorf("no cut decision for ADR-0007:\n%+v", b.Decisions)
	}
}

// A prompt no unit claims delivers the related lines alone: no unit line
// is faked, the decisions carry the refusal, and the brief still renders.
func TestRoles_NoClaimDeliversTheRelatedLinesAlone(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "fix org expiry daily cache purge", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	if b.Roles.Found() || !strings.HasPrefix(b.Roles.Why, "refused: ") {
		t.Fatalf("roles %+v", b.Roles)
	}
	if b.Empty() {
		t.Fatal("no brief")
	}
	for _, l := range b.Lines {
		if l.Role != "" {
			t.Errorf("a role line without a unit: %s %s", l.Role, l.Key)
		}
	}
}

func containsID(ids []graph.ID, id graph.ID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
