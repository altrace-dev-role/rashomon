package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The R0 ladder tests (W-M1.13): a prompt word in name form (a path, an
// identifier, a knowledge id, a scope tag) is a claim at the top of the
// ladder, each shape one prompt the edge set missed at d527ed53.

func namesFixture(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"jobs", "jobs/purge_sessions.py", []synSym{fn("purge_sessions"), fn("decode"), fn("drop_fact")}},
		{"orgs", "orgs/settings.py", []synSym{fn("get_org_settings"), fn("org_timeout"), fn("Impact")}},
		{"sessions", "sessions/expiry.py", []synSym{fn("expire_session"), fn("session_ttl"), fn("decode"), fn("drop_fact")}},
	}, [][2]string{{"jobs/purge_sessions.py#purge_sessions", "sessions/expiry.py#expire_session"}}, nil)
	record(t, g, "req:PF-X1", "requirement", "orgs/settings.py")
	return g
}

// A path the index holds names its file and unit outright (J4 shape).
func TestNames_APathNamesItsFileAndUnit(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Fix orgs/settings.py so the override is read once.")
	if r.Why != ClassNamed || r.Path != "orgs" || string(r.File) != "file:orgs/settings.py" {
		t.Fatalf("why %q path %q file %q, want a named claim on orgs via its file", r.Why, r.Path, r.File)
	}
	if w := r.Named.Words; len(w) != 1 || w[0].Kind != NamedPath || w[0].Unit != "orgs" {
		t.Errorf("named words %+v", w)
	}
}

// A path the index lacks resolves to the unit of its directory, with the
// gap disclosed (J1 shape: a test file is not indexed, its package is).
func TestNames_AnUnindexedPathNamesItsDirectory(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Fix jobs/purge_sessions_test.py so the race detector passes.")
	if r.Why != ClassNamed || r.Path != "jobs" || r.File != "" {
		t.Fatalf("why %q path %q file %q, want jobs by directory and no file", r.Why, r.Path, r.File)
	}
	if w := r.Named.Words[0]; w.Node != "" || !strings.Contains(w.Why, "not in the index") {
		t.Errorf("word %+v, want the missing file disclosed", w)
	}
}

// A path form the index has no unit for refuses the ladder: the request
// names a file the graph cannot own, and a phrase claim over the leftover
// words would be a false positive (G1 shape: a root README). What counts
// as a file name is read from the index's own extensions, never a list.
func TestNames_AnUnknownPathRefusesTheLadder(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Update cleanup.py so the org settings section mentions the timeout.")
	if r.Found() || !strings.Contains(r.Why, "cleanup.py") {
		t.Fatalf("unit %q why %q, want no unit and the unknown path named", r.Path, r.Why)
	}
	// An extension no indexed file has is not a path form: the word is an
	// English word to the frame, and the phrase ladder reads the rest.
	r = roles(t, g, "Update README.md so the org settings section mentions the timeout.")
	if len(r.Named.Words) != 0 {
		t.Fatalf("named %+v, want no named word", r.Named.Words)
	}
}

// An identifier (an underscore, a case change) names its symbol, its file
// and its unit (J2 shape); one held by two units resolves by the rest of
// the prompt's words against each, else stands disclosed and unclaimed. A
// bare lower-case word is an English word even when a symbol shares it
// ("decode", "report", "scan"): the phrase ladder reads those.
func TestNames_AnIdentifierNamesItsSymbol(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "expire_session double counts when two sessions share a ttl; fix it.")
	if r.Why != ClassNamed || r.Path != "sessions" || string(r.File) != "file:sessions/expiry.py" || len(r.Targets) != 1 {
		t.Fatalf("why %q path %q file %q targets %v", r.Why, r.Path, r.File, r.Targets)
	}
	r = roles(t, g, "drop_fact drops empty facts; keep them.")
	if r.Named.Unit != "" || len(r.Named.Words) != 1 || !strings.Contains(r.Named.Words[0].Why, "2 units") {
		t.Errorf("ambiguous identifier: named %+v, want it disclosed and unclaimed", r.Named)
	}
	r = roles(t, g, "drop_fact drops empty facts when a session expires; keep them.")
	if r.Why != ClassNamed || r.Path != "sessions" {
		t.Errorf("identifier held by two units, prompt names one: why %q path %q, want sessions", r.Why, r.Path)
	}
	r = roles(t, g, "decode drops empty facts; keep them.")
	if len(r.Named.Words) != 0 {
		t.Errorf("bare lower-case word read as a name: %+v", r.Named.Words)
	}
	r = roles(t, g, "The function Impact drops facts whose org is empty.")
	if r.Why != ClassNamed || r.Path != "orgs" {
		t.Errorf("capitalised mid-sentence word: why %q path %q, want orgs", r.Why, r.Path)
	}
}

// A qualified name (unit.symbol) resolves inside the unit it names (J3 shape).
func TestNames_AQualifiedNameResolvesInItsUnit(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "sessions.decode should report the elapsed time separately.")
	if r.Why != ClassNamed || r.Path != "sessions" || len(r.Targets) != 1 || !strings.HasSuffix(string(r.Targets[0]), "expiry.decode") {
		t.Fatalf("why %q path %q targets %v", r.Why, r.Path, r.Targets)
	}
}

// A knowledge id names its record: the record leads the knowledge role,
// and when the words claim no unit the record's evidence does (N1 shape).
func TestNames_AKnowledgeIDNamesItsRecord(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Implement PF-X1.")
	if len(r.Knowledge) == 0 || string(r.Knowledge[0]) != "req:PF-X1" {
		t.Fatalf("knowledge %v, want req:PF-X1 first", r.Knowledge)
	}
	if r.Path != "orgs" || !strings.Contains(r.Why, "PF-X1") {
		t.Errorf("why %q path %q, want orgs through the record's evidence", r.Why, r.Path)
	}
}

// A scope tag ("the jobs package") names the unit whose last path segment
// it is (E2 shape).
func TestNames_AScopeTagNamesTheUnit(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "The flaky test in the jobs package fails under -race; make it deterministic.")
	if r.Why != ClassNamed || r.Path != "jobs" {
		t.Fatalf("why %q path %q, want jobs by its tag", r.Why, r.Path)
	}
}

// A name inside a constraint clause is what not to touch, so it claims
// nothing (O1 shape); the first name outside one wins.
func TestNames_AConstraintClauseNamesNothing(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Do not touch orgs/settings.py; change only sessions/expiry.py so the ttl is read once.")
	if r.Why != ClassNamed || r.Path != "sessions" {
		t.Fatalf("why %q path %q, want sessions (orgs is excluded)", r.Why, r.Path)
	}
}

// The anchor phrase (a definite phrase the prompt marks as already
// existing) wins over a name when they disagree, and the disagreement is
// disclosed (W-M1.13's yield).
func TestNames_TheAnchorWinsOverANameWithDisclosure(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Add the ttl to sessions/expiry.py wherever the org settings already appear.")
	if r.Path != "orgs" || !strings.Contains(r.Why, "sessions/expiry.py") {
		t.Fatalf("why %q path %q, want orgs by the anchor with the name disclosed", r.Why, r.Path)
	}
}

// The frame keeps an identifier's case so its tokens split (J3 shape:
// verify.EvaluateStop is three tokens, not one unknown word).
func TestFrame_AnIdentifierKeepsItsCase(t *testing.T) {
	f := Parse("sessions.ExpireSession should report the fold step's time separately.")
	if len(f.Targets) == 0 || strings.Join(f.Targets[0].Tokens, " ") != "session expir" {
		t.Fatalf("targets %v, want [session expir]", f.Targets)
	}
}
