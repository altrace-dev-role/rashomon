package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestVerdict_NamesOwedGateAfterTouchedUnit: contracts-v2 section 3.3 and
// the trace's Stop. After the three edits and one gate run, the verdict
// owes the two gates no run has paid, each with a basis of certain edges,
// and carries the revision it was computed at. The 11-04a Obligation had
// no basis field.
func TestVerdict_NamesOwedGateAfterTouchedUnit(t *testing.T) {
	f := newFixture(t, retention{})
	f.editsThroughPurge(t)
	f.Ran(18, "pytest services/auth", true)
	v := f.Stop(20)
	if v.Done {
		t.Fatalf("done with gates owed:\n%s", v.Text)
	}
	if len(v.Refused) != 0 {
		t.Fatalf("refused: %v", v.Refused)
	}
	var jobs *Obligation
	for i := range v.Owed {
		if v.Owed[i].Cmd == "pytest jobs" {
			jobs = &v.Owed[i]
		}
	}
	if jobs == nil {
		t.Fatalf("jobs gate not owed:\n%s", v.Text)
	}
	if len(jobs.Basis) == 0 {
		t.Fatal("obligation without a basis")
	}
	for _, b := range jobs.Basis {
		e, ok := f.Store.Edge(b.Src, b.Pred, b.Dst, v.Rev)
		if !ok || !e.Certain {
			t.Fatalf("obligation basis %+v is not certain (ok=%v)", e, ok)
		}
	}
	for _, w := range []string{
		"rashomon: not done (rev C1+ov16).",
		"Owed: pytest jobs (jobs/purge_sessions.py written at call 16, no run since); pytest tests/integration -k session (jobs/purge_sessions.py written at call 16, no run since).",
		"R1 necessary condition met; R2 met.",
	} {
		if !strings.Contains(v.Text, w) {
			t.Fatalf("verdict lacks %q:\n%s", w, v.Text)
		}
	}
	if strings.Contains(v.Text, "pytest services/auth (") {
		t.Fatalf("a paid gate was owed:\n%s", v.Text)
	}
}

// TestVerdict_ObligationNeverFromUncertainEdge: the session's one
// obligation writer refuses a may_call basis.
func TestVerdict_ObligationNeverFromUncertainEdge(t *testing.T) {
	f := newFixture(t, retention{})
	sweep := graphtest.Sym("jobs", "jobs.nightly.sweep")
	create := graphtest.Sym("services/auth", "auth.sessions.create_session")
	f.Store.AssertEdge(graph.Edge{Src: sweep, Pred: graph.MayCall, Dst: create, Tier: graph.Algorithmic, Conf: 0.6, Rev: graph.RevCommit(graphtest.Commit)})
	_, err := NewObligation(f.Store, f.Rev(), "pytest jobs", "may call", []graph.EdgeRef{{Src: sweep, Pred: graph.MayCall, Dst: create}})
	if !errors.Is(err, graph.ErrUncertainBasis) {
		t.Fatalf("writer accepted an uncertain basis: %v", err)
	}
	unit := graph.UnitID("py", "jobs")
	o, err := NewObligation(f.Store, f.Rev(), "pytest jobs", "written", []graph.EdgeRef{{Src: unit, Pred: graph.Contains, Dst: graph.FileID(purgePy)}})
	if err != nil || o.Cmd != "pytest jobs" {
		t.Fatalf("certain basis refused: %v", err)
	}
}

// TestVerdict_SignatureConflictOwedAtStop: contracts-v2 section 2.3. A
// signature edit's conflicts stay pending, nothing is delivered at the
// act, a caller edit resolves its own, and the untouched caller is owed
// at Stop with its file, line and revision.
func TestVerdict_SignatureConflictOwedAtStop(t *testing.T) {
	f := newFixture(t, retention{})
	f.edit(t, 12, sessionsPy, "def create_session(user, org):", "def create_session(user, org, ttl):")
	if n := len(f.ConflictsIn(Pending)); n != 2 {
		t.Fatalf("pending after the signature edit = %d, want 2", n)
	}
	f.edit(t, 14, "services/auth/auth/api.py", "create_session(user, org)", "create_session(user, org, ttl=settings.session_timeout_s)")
	if p, r := f.ConflictsIn(Pending), f.ConflictsIn(Resolved); len(p) != 1 || len(r) != 1 {
		t.Fatalf("after the caller edit: pending %d resolved %d, want 1 and 1", len(p), len(r))
	}
	v := f.Stop(20)
	if len(f.ConflictsIn(Owed)) != 1 || v.Done {
		t.Fatalf("conflict not owed at Stop:\n%s", v.Text)
	}
	want := "create_session signature is (user, org, ttl) (rev C1+ov14); caller tests/unit/test_sessions.py:5 still passes 2 arguments (rev C1)."
	if !strings.Contains(v.Text, want) {
		t.Fatalf("verdict lacks %q:\n%s", want, v.Text)
	}
}

// TestLabel_NotFollowedLocalGreenDoesNotValidateLesson: contracts-v2
// section 4.3. A delivered line the agent did not act on, with every gate
// green locally, is labelled partial with a local outcome; a local
// outcome never counts for a lesson; a hidden red does. The 11-04a label
// was steer.Label alone, so a green run would have refuted the line.
func TestLabel_NotFollowedLocalGreenDoesNotValidateLesson(t *testing.T) {
	f := newFixture(t, retention{})
	f.editsSkippingCache(t)
	f.Ran(18, "pytest services/auth", true)
	f.Ran(19, "pytest jobs", true)
	f.Ran(20, "pytest tests/integration -k session", true)
	v := f.Stop(30)
	if !v.Done {
		t.Fatalf("not done with every gate paid:\n%s", v.Text)
	}
	for _, w := range []string{
		"rashomon: done (rev C1+ov16).",
		"Not acted on: brief line 1 (auth/cache.py put_session ex=3600 unchanged; no gate covers it; not owed).",
		"Recorded: line 1 followed=partial, outcome=green_local; line 2 followed=yes, outcome=green_local.",
	} {
		if !strings.Contains(v.Text, w) {
			t.Fatalf("verdict lacks %q:\n%s", w, v.Text)
		}
	}
	d1 := f.Delivered[0]
	if d1.Label != (Followed{Label: FollowedPartial, Outcome: OutcomeGreenLocal, JudgedBy: "predicate"}) {
		t.Fatalf("line 1 label = %+v", d1.Label)
	}
	if CountsForLesson(d1.Label.Outcome) {
		t.Fatal("a local green counted for a lesson")
	}
	f.IngestHidden(map[string]bool{"redis_ttl_matches": false, "org_override_applied": true})
	if d1.Label.Outcome != OutcomeRedHidden || d1.Label.JudgedBy != "hidden" {
		t.Fatalf("hidden red not recorded: %+v", d1.Label)
	}
	if !CountsForLesson(d1.Label.Outcome) {
		t.Fatal("a hidden red must count for a lesson")
	}
	if f.Delivered[1].Label.Outcome != OutcomeGreenLocal {
		t.Fatalf("a line naming no hidden case moved: %+v", f.Delivered[1].Label)
	}
}
