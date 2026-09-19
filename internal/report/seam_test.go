package report

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// THE SEAM TESTS.
//
// A review deleted `w.RunID = cfg.runToken` -- the one line that makes a minted
// tag a join key -- and the entire suite stayed green. Then the three lines
// copying the counters into Destinations: green. Then the writeJoin call site:
// green. Three separate deletions, each of which ships a feature that mints a
// tag, exports it, has the proxy record it, and then silently ignores all of
// it while rendering "no session token was in use", which reads as correct.
//
// Every test that existed drove either summarise() with a Window it built
// itself, or writeJoin() with a Destinations it built itself. Both ends were
// covered and the wire between them was not, which is the classic shape: unit
// tests on the leaves, nothing on the seam, and a feature that can be
// disconnected without a red light.
//
// These go through report.Build against a real sqlite store, which is the only
// path that proves the parts are joined.

func seamStore(t *testing.T, rows [][4]string, at time.Time) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "causal.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE causal_records (
		sequence_num INTEGER PRIMARY KEY,
		record_id TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT '',
		run_id TEXT NOT NULL DEFAULT '',
		timestamp DATETIME NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		target_host TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	stamp := at.UTC().Format("2006-01-02 15:04:05.999999999 -0700 MST")
	for i, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO causal_records (sequence_num, request_id, run_id, timestamp, action, target_host)
			 VALUES (?, ?, ?, ?, 'ALLOW', ?)`,
			i+1, r[0], r[1], stamp, r[2]); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestSeam_TheTokenReachesTheJoin is the one that goes red when
// `w.RunID = cfg.runToken` is deleted.
func TestSeam_TheTokenReachesTheJoin(t *testing.T) {
	st, sessionID, now := seamSession(t)
	db := seamStore(t, [][4]string{
		{"r1", "rt_ours", "pypi.org", ""},        // ours, exactly
		{"r2", "", "github.com", ""},             // git: no credential, clock decides
		{"r3", "rt_theirs", "other.example", ""}, // another run of this install
	}, now)

	rep, err := Build(st, sessionID, now,
		WithProxyStore(db),
		WithRunToken("rt_ours"),
		WithTokenVerifier(func(s string) bool { return strings.HasPrefix(s, "rt_") }),
	)
	if err != nil {
		t.Fatal(err)
	}
	d := rep.Sessions[0].Destinations

	// THE PREMISE. A window that does not contain the rows makes every
	// assertion below fail for a reason that has nothing to do with the join.
	if !d.Observed || !d.WindowApplied {
		t.Fatalf("premise: observed=%v windowApplied=%v reason=%q",
			d.Observed, d.WindowApplied, d.Reason)
	}
	// Inherited here should be EXACTLY the other-token row: inherited by tag,
	// not by time. Any excess means rows fell outside the window and the
	// fixture is wrong rather than the join.
	if d.Inherited != d.OtherToken {
		t.Fatalf("premise: inherited=%d other_token=%d -- %d row(s) fell outside the "+
			"window, so the fixture is wrong, not the join",
			d.Inherited, d.OtherToken, d.Inherited-d.OtherToken)
	}

	if d.TokenMatched != 1 {
		t.Errorf("token_matched = %d, want 1. The minted tag never reached the join: "+
			"the feature mints, exports and records, then ignores all of it.", d.TokenMatched)
	}
	if d.WindowMatched != 1 {
		t.Errorf("window_matched = %d, want 1 (the untokened row)", d.WindowMatched)
	}
	if d.OtherToken != 1 {
		t.Errorf("other_token = %d, want 1", d.OtherToken)
	}
	if !d.TokenRequested {
		t.Error("token_requested is false on a run that supplied a token")
	}
}

// TestSeam_TheJoinLineIsRenderedInARealReport goes red when the writeJoin call
// site is removed. The isolation test cannot: it calls writeJoin directly.
func TestSeam_TheJoinLineIsRenderedInARealReport(t *testing.T) {
	st, sessionID, now := seamSession(t)
	db := seamStore(t, [][4]string{
		{"r1", "rt_ours", "pypi.org", ""},
		{"r2", "", "github.com", ""},
	}, now)

	rep, err := Build(st, sessionID, now,
		WithProxyStore(db), WithRunToken("rt_ours"),
		WithTokenVerifier(func(s string) bool { return strings.HasPrefix(s, "rt_") }))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "join: token (1 request) + window (1 request)") {
		t.Errorf("the join line is absent from a real rendered report:\n%s", b.String())
	}
}

// TestSeam_AnUnverifiableTagIsNotAnotherSession is the security fix at the
// seam: the agent forges a credential, and its destination must survive.
func TestSeam_AnUnverifiableTagIsNotAnotherSession(t *testing.T) {
	st, sessionID, now := seamSession(t)
	db := seamStore(t, [][4]string{
		{"r1", "rt_ours", "pypi.org", ""},
		{"r2", "forged-by-the-agent", "exfil.example", ""},
	}, now)

	rep, err := Build(st, sessionID, now,
		WithProxyStore(db), WithRunToken("rt_ours"),
		// Only tags we signed verify. The forged one does not.
		WithTokenVerifier(func(s string) bool { return s == "rt_ours" || s == "rt_sibling" }))
	if err != nil {
		t.Fatal(err)
	}
	d := rep.Sessions[0].Destinations

	if d.OtherToken != 0 {
		t.Errorf("other_token = %d; a tag we never issued was counted as another session",
			d.OtherToken)
	}
	var sawExfil bool
	for _, h := range d.Hosts {
		if h.Host == "exfil.example" {
			sawExfil = true
			if h.Attempts == 0 {
				t.Error("the forged row was excluded from this session's attempts, which " +
					"removes exfil.example from the finding the product exists to make")
			}
		}
	}
	if !sawExfil {
		t.Errorf("exfil.example is absent from the destinations entirely: %+v", d.Hosts)
	}
}

// TestSeam_ATokenThatNoRowCarriedIsADiagnostic. A tag was in play and nothing
// carried it: either the proxy is not writing run_id or every client stripped
// the credential. Reporting that as "no session token was in use" states the
// failure as its own opposite.
func TestSeam_ATokenThatNoRowCarriedIsADiagnostic(t *testing.T) {
	st, sessionID, now := seamSession(t)
	db := seamStore(t, [][4]string{{"r1", "", "github.com", ""}}, now)

	rep, err := Build(st, sessionID, now,
		WithProxyStore(db), WithRunToken("rt_ours"),
		WithTokenVerifier(func(string) bool { return false }))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "no session token was in use") {
		t.Errorf("a tokened run reported that no token was in use:\n%s", out)
	}
	if !strings.Contains(out, "NO row carried it") {
		t.Errorf("the diagnostic is missing:\n%s", out)
	}
}

// seamSession builds a minimal recorded session whose window contains now.
func seamSession(t *testing.T) (*store.Store, string, time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	const id = "seam-session"
	// The window must CONTAIN the rows. The first version of this helper put
	// both the start and the end a minute in the past, so the window closed
	// before the fixture rows existed and every one of them read as inherited
	// -- four failing seam tests that looked like a broken join and were a
	// broken fixture. Asserted below rather than left to be inferred.
	for phase, at := range map[string]time.Time{
		store.PhaseStart: now.Add(-time.Minute),
		store.PhaseEnd:   now.Add(time.Minute),
	} {
		if err := st.AppendCoverage(store.Coverage{
			Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: phase,
			RecordedAtMS: at.UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, id, now
}

// TestSeam_TheNonoTrailReachesTheReport.
//
// Written because the adapter was, for a while, exactly what it warns about:
// a package that compiled, had nine green tests against a real capture, and
// was never called. buildNono existed, WithNonoTrail existed, the renderer
// existed, and Build never invoked any of it -- so the report printed
// "sandbox (nono): not observed (unknown)" on a session with a perfectly good
// trail, which reads as an honest answer.
//
// This goes red the moment that wire is cut again.
func TestSeam_TheNonoTrailReachesTheReport(t *testing.T) {
	// THE WINDOW MUST CONTAIN THE FIXTURE'S INSTANTS. They are real wall-clock
	// times from the machine that recorded the capture, not relative offsets,
	// so a window built around time.Now() excludes all four events -- which
	// reads as "the adapter is not wired" and is not.
	at := firstNonoEvent(t)
	st, sessionID, now := seamSessionAt(t, at)

	rep, err := Build(st, sessionID, now, WithNonoTrail(nonoFixture))
	if err != nil {
		t.Fatal(err)
	}
	n := rep.Sessions[0].Nono

	if !n.Observed {
		t.Fatalf("the trail was not read: %q. The adapter is not wired into Build.", n.Reason)
	}
	if len(n.Allowed) == 0 {
		t.Error("no allowed hosts reached the report from a trail that has two")
	}
	if len(n.Denied) == 0 {
		t.Error("no denied hosts reached the report from a trail that has two")
	}

	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "sandbox (nono):") {
		t.Errorf("the sandbox line is absent from a rendered report:\n%s", b.String())
	}
	// Scoped to the sandbox line. An unscoped match catches "destinations: not
	// observed (no_proxy_store)", which is a different section being honest.
	if strings.Contains(b.String(), "sandbox (nono): not observed") {
		t.Errorf("a readable trail rendered as not observed:\n%s", b.String())
	}
}

// TestSeam_NoTrailConfiguredIsSilent.
//
// This asserted the OPPOSITE when it was written -- that the section always
// prints, on the rule that a section which vanishes cannot be told apart from
// one that was never built. H-28 caught it: that rule is right for a source
// the session was USING and wrong for an optional one it was not. Printing
// "not observed" for a sandbox nobody asked for puts a degradation marker on
// every healthy run, and a reader who meets the word there stops believing it.
func TestSeam_NoTrailConfiguredIsSilent(t *testing.T) {
	st, sessionID, now := seamSession(t)
	rep, err := Build(st, sessionID, now)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	// SILENT, and that is the corrected rule. H-28 asserts a healthy run shows
	// no degradation marker, and most sessions run no sandbox at all -- so
	// "not observed" here would print that marker on every clean session and
	// teach readers to discount the word.
	if strings.Contains(b.String(), "sandbox (nono)") {
		t.Errorf("a session with no sandbox configured rendered a sandbox line:\n%s",
			b.String())
	}
}

const nonoFixture = "../../test/fixtures/nono/audit-events.ndjson"

// firstNonoEvent reads the fixture's earliest instant, so a window can be
// built around data rather than around the clock.
func firstNonoEvent(t *testing.T) time.Time {
	t.Helper()
	obs := nono.Read(nonoFixture, nono.Window{Start: time.Unix(0, 0)})
	if !obs.Observed || len(obs.Events) == 0 {
		t.Fatalf("premise: the nono fixture yields no events (%s)", obs.Reason)
	}
	return obs.Events[0].At
}

// seamSessionAt is seamSession with the window centred on a given instant.
func seamSessionAt(t *testing.T, at time.Time) (*store.Store, string, time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "seam-session"
	for phase, when := range map[string]time.Time{
		store.PhaseStart: at.Add(-time.Minute),
		store.PhaseEnd:   at.Add(time.Hour),
	} {
		if err := st.AppendCoverage(store.Coverage{
			Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: phase,
			RecordedAtMS: when.UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, id, at.Add(time.Hour)
}

// TestSeam_TheReconciliationActuallyExecutes.
//
// The review's sharpest finding was not a bug but a DEAD REGION: the only test
// reaching buildNono passed no proxy store, so dests.Observed was false, the
// function returned at its first guard, and the entire reconciliation --
// both disagreement columns, the plain-HTTP explanation, the denied-host
// exclusion -- never ran in any test in the repo. Eleven mutations survived
// inside it.
//
// This supplies BOTH sides, so the region executes, and pins the three
// defects that were living in it.
func TestSeam_TheReconciliationActuallyExecutes(t *testing.T) {
	at := firstNonoEvent(t)
	st, id, now := seamSessionAt(t, at)

	// The wire: one host both sides saw, one the sandbox refused, one that is
	// loopback, one that is the client's own plane.
	db := seamStore(t, [][4]string{
		{"r1", "", "pypi.org", ""},
		{"r2", "", "github.com", ""},
		{"r3", "", "localhost", ""},
		{"r4", "", "api.anthropic.com", ""},
	}, at)

	rep, err := Build(st, id, now, WithNonoTrail(nonoFixture), WithProxyStore(db))
	if err != nil {
		t.Fatal(err)
	}
	n := rep.Sessions[0].Nono

	if !n.Observed {
		t.Fatalf("premise: trail not observed (%s)", n.Reason)
	}

	// github.com is DENIED by the sandbox and present on the wire. It must not
	// appear in both columns -- the fixture's own data used to produce
	// "refused by the sandbox: github.com" and "on the wire and not in the
	// sandbox's trail: github.com" in the same section.
	for _, h := range n.ProxySawWhatItDidNot {
		if h == "github.com" {
			t.Error("github.com is listed as missing from the trail AND refused by it. " +
				"The proxy records the attempt regardless of the sandbox's verdict, so a " +
				"denial is agreement, not a gap.")
		}
	}

	// Loopback and the client plane must not read as traffic the sandbox
	// missed. They are the categories the rest of this package argues at
	// length must never read as findings.
	for _, h := range n.ProxySawWhatItDidNot {
		if h == "localhost" || h == "api.anthropic.com" {
			t.Errorf("%s is in the sandbox-missed column; it is %s", h,
				map[string]string{
					"localhost":         "loopback, which is never proxied",
					"api.anthropic.com": "the client's own plane",
				}[h])
		}
	}

	// And the region really ran: example.com is allowed in the trail and
	// absent from this wire fixture, so the other column must be non-empty.
	var sawExample bool
	for _, h := range n.SawWhatTheProxyDidNot {
		if h == "example.com" {
			sawExample = true
		}
	}
	if !sawExample {
		t.Errorf("saw_what_the_proxy_did_not = %v; the reconciliation did not run",
			n.SawWhatTheProxyDidNot)
	}
}

// TestSeam_AForgottenHostDoesNotReturnUnderTheSandboxHeading.
//
// buildNono read the Destinations view for the stated reason that it has
// "already suppressed forgotten hosts" -- true of the wire side only. The
// trail is a second place the name lives, and leaving it unfiltered did worse
// than republish: suppression removes the host from the wire side, so the
// forgotten host became the one thing the section calls out. Forgetting
// PROMOTED it.
func TestSeam_AForgottenHostDoesNotReturnUnderTheSandboxHeading(t *testing.T) {
	at := firstNonoEvent(t)
	st, id, now := seamSessionAt(t, at)

	// A DECLARATION NAMING THE HOST, because forget --host writes a gap only
	// for runs where it evicted something -- and the predicate that drives
	// suppression is built from those gaps. Without this the forget is a
	// silent no-op and the test passes for the wrong reason.
	//
	// THAT IS ALSO A PRODUCT FINDING, recorded here because the test is where
	// it was found: a host that appears ONLY in the sandbox's trail and never
	// in rashomon's own records cannot be forgotten at all. `forget --host`
	// forgets what rashomon recorded; the fourth evidence source has a hole.
	if err := st.AppendDeclaration(store.Declaration{
		Type: "declaration", SchemaVersion: 2, SessionID: id,
		ToolUseID: "toolu_forget", ToolName: "Bash",
		RecordedAtMS: at.UnixMilli(), Hosts: []string{"pypi.org"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgetHost("pypi.org", now); err != nil {
		t.Fatalf("forget --host: %v", err)
	}
	forgotten, err := st.ForgottenHost()
	if err != nil {
		t.Fatal(err)
	}
	if !forgotten("pypi.org") {
		t.Fatal("premise: the forget wrote no gap, so nothing is suppressed and this " +
			"test would pass for the wrong reason")
	}

	// A PROXY STORE, because without one buildNono returns at its first guard
	// and never reaches the code this test exists to pin. The first version
	// omitted it: deleting suppressTrail entirely left this test -- and the
	// whole suite -- green. A test that cannot fail when its subject is
	// removed is not pinning anything, and this one was written for a defect
	// that had already shipped once.
	db := seamStore(t, [][4]string{{"r1", "", "example.com", ""}}, at)

	rep, err := Build(st, id, now, WithNonoTrail(nonoFixture), WithProxyStore(db))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "pypi.org") {
		t.Errorf("a forgotten host returned under the sandbox heading:\n%s", b.String())
	}
	// And the premise: the section must have RENDERED, or the absence above
	// proves only that nothing was printed at all.
	if !strings.Contains(b.String(), "sandbox (nono):") {
		t.Fatalf("premise: the sandbox section did not render, so this test proves "+
			"nothing:\n%s", b.String())
	}
}
