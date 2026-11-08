package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestRoles_TaskANamesTheFourRoles (W-M1.12, AC-M1.1's shape): for Task A
// the brief's first five lines are the unit that owns the task (the
// target is "session expiry", what "respect" opens is the use phrase), the
// file it enters by, the abstraction to reuse (the org settings accessor
// names a word of the use phrase; "session" and "expiry" named the target
// and are spent, so insert_session rests on a doc mention alone and
// ranks after it; the daily report's doc mention of org_id outside the
// unit is no reuse), the consumer outside the unit, and the knowledge
// that applies: the unit's own lesson, reached through the table its db
// module writes (W-M1.23); the decision on the reuse is the reuse line's
// to show, never a knowledge line.
func TestRoles_TaskANamesTheFourRoles(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "make session expiry respect the org-level timeout override.", Options{IDF: true, bands: stateAll})
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
	if len(r.Knowledge) != 1 || r.Knowledge[0] != "les:INC-212" {
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
		"unit 'services/auth' (by word coverage; runner-up '" + r.Picks[1].Path + "') declares 'is_expired', 'create_session', 'SESSION_TTL_SECONDS'; is consumed by 'jobs/purge_sessions.py', 'tests/unit/test_sessions.py' and 1 more",
		"'services/auth/auth/sessions.py' declares",
		"reuse 'services/orgs/orgs/settings.py' (named to use) declares 'get_org_settings'; imports 'orgs.models'; ADR-0007 applies",
		"consumer 'jobs/purge_sessions.py'",
		"lesson INC-212 applies to 'jobs/purge_sessions.py'",
	}
	for i, want := range wants {
		if i >= len(lines) || !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d\n got %q\nwant prefix %q", i+1, lines[i], want)
		}
	}
	for _, d := range b.Decisions {
		if d.ID == "dec:ADR-0007" && strings.HasPrefix(d.Why, "role:") {
			t.Errorf("the reuse's record holds a role line: %+v", d)
		}
	}
}

// A prompt no unit claims delivers the related lines alone: no unit line
// is faked, the decisions carry the refusal, and the brief still renders.
// Such a brief is read by its first line's unit, a claim the calibration
// withholds (W-M1.38, BasisNone), so the not-known line opens it in the
// unit's slot and names the unit alone; the finding's source is the first
// related line.
func TestRoles_NoClaimDeliversTheRelatedLinesAlone(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "fix org expiry daily cache purge", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	if b.Roles.Found() || !strings.HasPrefix(b.Roles.Why, "refused: ") {
		t.Fatalf("roles %+v", b.Roles)
	}
	if b.Empty() || len(b.Lines) < 2 {
		t.Fatalf("no brief: %d lines, %d kept", len(b.Lines), b.Kept)
	}
	nk, ok := b.Lines[0].Slot.NotKnown()
	if b.Lines[0].Role != RoleNotKnown || !ok || !slices.Equal(nk.Unknown, []string{slot.UnknownUnit}) {
		t.Errorf("line 1: %+v, want the not-known line naming the unit alone", b.Lines[0])
	}
	if lines := strings.Split(b.Text(), "\n"); len(lines) < 3 || lines[1] != "not known: the unit to change" {
		t.Errorf("the brief's first line is not the not-known line: %q", lines)
	}
	if b.FirstKey() != b.Lines[1].Key || b.FirstKey() == "" {
		t.Errorf("first key %q, want line 2's %q", b.FirstKey(), b.Lines[1].Key)
	}
	for _, l := range b.Lines[1:] {
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

// TestRoles_TheUnitLineCarriesTheClaimsBasisAndRunnerUp (S3, AC-M1.1
// clause 3): the unit line the roles stage delivers says how the unit was
// claimed: Task A's unit is claimed by word coverage, so its line carries
// the coverage basis and names the ladder's runner-up, the next unit by
// score; a request that names the unit's path is claimed by name, so its
// line carries the named basis and no runner-up. The basis and the
// runner-up travel on the engine's Line and into the slot's fact.
func TestRoles_TheUnitLineCarriesTheClaimsBasisAndRunnerUp(t *testing.T) {
	sp := graphtest.New(t)
	withKnowledge(sp)
	rev := graph.RevCommit(graphtest.Commit)
	b := Build(sp.Store, rev, "make session expiry respect the org-level timeout override.", Options{IDF: true, bands: stateAll})
	t.Logf("\n%s", b.Explain())
	r := b.Roles
	if r.Why != ClassFull || len(r.Picks) < 2 {
		t.Fatalf("Task A: why %q, picks %d", r.Why, len(r.Picks))
	}
	if r.Basis != BasisCoverage || r.RunnerUp != r.Picks[1].ID {
		t.Errorf("roles basis %q runner-up %q; want %q and %q", r.Basis, r.RunnerUp, BasisCoverage, r.Picks[1].ID)
	}
	if l := b.Lines[0]; l.Role != RoleUnit || l.Basis != BasisCoverage || l.RunnerUp != r.Picks[1].ID {
		t.Errorf("unit line basis %q runner-up %q", l.Basis, l.RunnerUp)
	}
	u, ok := b.Lines[0].Slot.Unit()
	if !ok || u.Basis != slot.BasisCoverage || string(u.RunnerUp) != r.Picks[1].Path {
		t.Errorf("slot unit fact %+v; want coverage and runner-up %q", u, r.Picks[1].Path)
	}
	if !strings.Contains(b.Text(), "unit 'services/auth' (by word coverage; runner-up '"+r.Picks[1].Path+"')") {
		t.Errorf("brief text lacks the disclosure:\n%s", b.Text())
	}
	b = Build(sp.Store, rev, "in services/auth, make session expiry respect the org-level timeout override.", Options{IDF: true, bands: stateAll})
	t.Logf("\n%s", b.Explain())
	if b.Roles.Why != ClassNamed || b.Roles.Basis != BasisNamed || b.Roles.RunnerUp != "" {
		t.Fatalf("named: why %q basis %q runner-up %q", b.Roles.Why, b.Roles.Basis, b.Roles.RunnerUp)
	}
	if !strings.Contains(b.Text(), "unit 'services/auth' (named in the request)") {
		t.Errorf("named brief text lacks the disclosure:\n%s", b.Text())
	}
}
