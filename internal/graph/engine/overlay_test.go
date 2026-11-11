package engine

import (
	"errors"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// The specimen's symbols.
var (
	createSession = graphtest.Sym("services/auth", "auth.sessions.create_session")
	handler       = graphtest.Sym("services/auth", "auth.api.create_session_handler")
	testCreate    = graphtest.Sym("tests", "tests.unit.test_sessions.test_create")
	getOrg        = graphtest.Sym("services/orgs", "orgs.settings.get_org_settings")
)

const (
	sessionsPy = "services/auth/auth/sessions.py"
	apiPy      = "services/auth/auth/api.py"
	cachePy    = "services/auth/auth/cache.py"
	oldDef     = "def create_session(user, org):"
	newDef     = "def create_session(user, org, ttl):"
)

// TestOverlay_CallerEditSeesCalleeOverlaySignature: contracts-v2 section
// 2.3. The signature edit at act 12 writes an overlay; the reader at
// {C1, 12} answers the new signature while the commit view keeps the old;
// the two committed callers conflict at ov12; the caller edit at act 14 is
// compared against the overlay signature, so api.py's conflict clears and
// only the untouched test caller remains. The 11-04a trace read C1 at act
// 14 and would have reported a conflict against a signature that no longer
// existed.
func TestOverlay_CallerEditSeesCalleeOverlaySignature(t *testing.T) {
	sp := graphtest.New(t)
	c1 := graph.RevCommit(graphtest.Commit)
	if n, ok := sp.Store.Node(createSession, c1); !ok || n.Attrs["sig"] != "(user, org)" {
		t.Fatalf("C1 create_session = %+v ok=%v", n, ok)
	}
	if _, err := sp.Overlay(sessionsPy, oldDef, newDef, 12); err != nil {
		t.Fatal(err)
	}
	ov12 := graph.RevOverlay(graphtest.Commit, 12)
	n, ok := sp.Store.Node(createSession, ov12)
	if !ok || n.Attrs["sig"] != "(user, org, ttl)" || n.Attrs["arity_min"] != "3" {
		t.Fatalf("create_session at ov12 = %+v ok=%v", n, ok)
	}
	if n, _ := sp.Store.Node(createSession, c1); n.Attrs["sig"] != "(user, org)" {
		t.Fatalf("overlay leaked into the commit view: %+v", n)
	}
	got := ArityConflicts(sp.Store, createSession, ov12)
	if len(got) != 2 || got[0].Edge.Src != handler || got[1].Edge.Src != testCreate {
		t.Fatalf("conflicts at ov12 = %s, want api.py and test_sessions.py", describe(got))
	}
	for _, c := range got {
		if c.Got != 2 || c.Want != "(user, org, ttl)" || c.Rev != ov12 || c.Edge.Attrs["line"] == "" {
			t.Fatalf("conflict = %+v", c)
		}
	}
	if c := ArityConflicts(sp.Store, createSession, c1); len(c) != 0 {
		t.Fatalf("conflicts in the commit view = %s, want none", describe(c))
	}

	if _, err := sp.Overlay(apiPy, "create_session(user, org)", "create_session(user, org, ttl=settings.session_timeout_s)", 14); err != nil {
		t.Fatal(err)
	}
	ov14 := graph.RevOverlay(graphtest.Commit, 14)
	got = ArityConflicts(sp.Store, createSession, ov14)
	if len(got) != 1 || got[0].Edge.Src != testCreate || got[0].Edge.Rev != c1 {
		t.Fatalf("conflicts at ov14 = %s, want test_sessions.py alone, its edge still at C1", describe(got))
	}
	if cc := CallerConflicts(sp.Store, handler, ov14); len(cc) != 0 {
		t.Fatalf("api.py still conflicts at ov14: %s", describe(cc))
	}
	e, ok := sp.Store.Edge(handler, graph.Calls, createSession, ov14)
	if !ok || e.Attrs["argc"] != "3" || e.Rev.Seq != 14 || e.Certain {
		t.Fatalf("api.py call edge at ov14 = %+v ok=%v", e, ok)
	}
}

// TestOverlay_CallerFirstThenSignatureResolves: the opposite order, same
// records, same outcome: the caller edit at act 12 conflicts against the
// committed signature; the signature edit at act 14 clears it.
func TestOverlay_CallerFirstThenSignatureResolves(t *testing.T) {
	sp := graphtest.New(t)
	if _, err := sp.Overlay(apiPy, "create_session(user, org)", "create_session(user, org, ttl)", 12); err != nil {
		t.Fatal(err)
	}
	ov12 := graph.RevOverlay(graphtest.Commit, 12)
	if cc := CallerConflicts(sp.Store, handler, ov12); len(cc) != 1 || cc[0].Got != 3 || cc[0].Want != "(user, org)" {
		t.Fatalf("caller-first conflicts at ov12 = %s", describe(cc))
	}
	if _, err := sp.Overlay(sessionsPy, oldDef, newDef, 14); err != nil {
		t.Fatal(err)
	}
	ov14 := graph.RevOverlay(graphtest.Commit, 14)
	if cc := CallerConflicts(sp.Store, handler, ov14); len(cc) != 0 {
		t.Fatalf("api.py conflicts after the signature overlay: %s", describe(cc))
	}
	if got := ArityConflicts(sp.Store, createSession, ov14); len(got) != 1 || got[0].Edge.Src != testCreate {
		t.Fatalf("conflicts at ov14 = %s, want test_sessions.py alone", describe(got))
	}
}

// TestOverlay_UnparseableEditWritesNothing: an edit that leaves a bracket
// open is refused by the reduced extractor; no overlay is written and the
// reader's view of the file is the last one it had.
func TestOverlay_UnparseableEditWritesNothing(t *testing.T) {
	sp := graphtest.New(t)
	_, err := sp.Overlay(cachePy, "ex=3600)", "ex=ttl", 14)
	if !errors.Is(err, pylite.ErrParse) {
		t.Fatalf("err = %v, want ErrParse", err)
	}
	if len(sp.Store.Overlays()) != 0 {
		t.Fatal("an overlay was written for unparsed content")
	}
	put := graphtest.Sym("services/auth", "auth.cache.put_session")
	if _, ok := sp.Store.Node(put, graph.RevOverlay(graphtest.Commit, 14)); !ok {
		t.Fatal("the last good view of cache.py was lost")
	}
}

// TestOverlay_RequirementPredicateHoldsAcrossEdits: R2, the constant
// unchanged from C1, and R1's necessary condition, create_session calls
// get_org_settings, are read at the overlay revision; the first holds
// before and after the edit, the second only after.
func TestOverlay_RequirementPredicateHoldsAcrossEdits(t *testing.T) {
	sp := graphtest.New(t)
	ttl := graphtest.Const("services/auth", "auth.sessions.SESSION_TTL_SECONDS")
	c1 := graph.RevCommit(graphtest.Commit)
	before, _ := sp.Store.Node(ttl, c1)
	if before.Attrs["value"] != "3600" {
		t.Fatalf("C1 constant = %+v", before)
	}
	if calls(sp.Store, createSession, getOrg, c1) {
		t.Fatal("R1's condition held at C1; the fixture is wrong")
	}
	_, err := sp.Overlay(sessionsPy,
		"from auth.db import insert_session\n",
		"from auth.db import insert_session\nfrom orgs.settings import get_org_settings\n", 10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Overlay(sessionsPy,
		"    expires_at = datetime.utcnow() + timedelta(seconds=SESSION_TTL_SECONDS)",
		"    ttl = get_org_settings(org.id).session_timeout_s or SESSION_TTL_SECONDS\n    expires_at = datetime.utcnow() + timedelta(seconds=ttl)", 12)
	if err != nil {
		t.Fatal(err)
	}
	ov12 := graph.RevOverlay(graphtest.Commit, 12)
	after, ok := sp.Store.Node(ttl, ov12)
	if !ok || after.Attrs["value"] != before.Attrs["value"] {
		t.Fatalf("R2 at ov12: constant = %+v ok=%v", after, ok)
	}
	if !calls(sp.Store, createSession, getOrg, ov12) {
		t.Fatalf("R1's condition does not hold at ov12: %+v", sp.Store.Out(createSession, ov12, graph.Calls))
	}
	if calls(sp.Store, createSession, getOrg, c1) {
		t.Fatal("the overlay call leaked into the commit view")
	}
}

// calls reports whether a calls edge src -> dst stands at rev.
func calls(r Reader, src, dst graph.ID, rev graph.Rev) bool {
	for _, e := range r.Out(src, rev, graph.Calls) {
		if e.Dst == dst {
			return true
		}
	}
	return false
}

// describe is conflicts as "src(got)->want@rev" for a failure message.
func describe(cs []Conflict) string {
	s := "["
	for _, c := range cs {
		s += string(c.Edge.Src) + " got " + strconv.Itoa(c.Got) + " want " + c.Want + " @" + c.Rev.String() + "; "
	}
	return s + "]"
}
