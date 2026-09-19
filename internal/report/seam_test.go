package report

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

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
