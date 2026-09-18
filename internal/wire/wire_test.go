package wire

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The timestamp layout the proxy actually writes, monotonic suffix included.
// Captured from a real causal.db on 2026-09-18:
//
//	2026-09-18 15:21:23.565097 -0400 EDT m=+7.991661334
//
// It is reproduced here rather than simplified, because a fixture in RFC 3339
// would make these tests pass against a reader that cannot parse the real
// thing -- which is the only shape that matters.
const proxyStamp = "2006-01-02 15:04:05.999999999 -0700 MST"

func stamp(t time.Time, monotonic float64) string {
	return fmt.Sprintf("%s m=+%.9f", t.Format(proxyStamp), monotonic)
}

type fixtureRow struct {
	seq       int64
	requestID string
	runID     string
	ts        string
	action    string
	reason    string
	host      string
}

// newStore writes a causal.db with the proxy's column set. Only the columns the
// reader selects are populated; the rest exist so the query is the same query.
func newStore(t *testing.T, rows []fixtureRow) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "causal.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
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
		t.Fatalf("create: %v", err)
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO causal_records (sequence_num, request_id, run_id, timestamp, reason, action, target_host)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.seq, r.requestID, r.runID, r.ts, r.reason, r.action, r.host); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return path
}

func find(obs Observation, host string) (Destination, bool) {
	for _, d := range obs.Hosts {
		if d.Host == host {
			return d, true
		}
	}
	return Destination{}, false
}

var base = time.Date(2026, 9, 18, 15, 21, 23, 0, time.UTC)

// TestRead_ParsesTheProxysOwnTimestampFormat is the test this package exists
// for. The proxy binds a time.Time to a DATETIME column, so the value lands as
// Go's String() rendering with the monotonic suffix attached, which no standard
// layout parses. A reader that quietly failed here would mark every row
// unusable, decline to apply the window, and still print the destinations --
// passing every other test in this file while silently attributing another
// session's traffic to this one.
func TestRead_ParsesTheProxysOwnTimestampFormat(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "req-1", ts: stamp(base.Add(time.Second), 7.99),
			action: "WARN", reason: "host_not_in_allowlist_observed", host: "pypi.org:443"},
	})

	obs := Read(path, Window{Start: base, End: base.Add(time.Minute)})

	if !obs.Observed {
		t.Fatalf("not observed: %s", obs.Reason)
	}
	if !obs.WindowApplied {
		t.Error("window was not applied, so the timestamp did not parse. The report " +
			"would then include rows from other sessions and could not say so.")
	}
	d, ok := find(obs, "pypi.org")
	if !ok {
		t.Fatalf("pypi.org missing from %+v", obs.Hosts)
	}
	if d.Inherited {
		t.Error("an in-window row was marked inherited")
	}
	if d.FirstSeen == "" {
		t.Error("first_seen is empty although the stamp parsed")
	}
}

// TestRead_PortStrippedAndCaseFolded pins that the wire side canonicalises with
// the same function as the declaration side. Without it the report's central
// comparison fails on exactly the hosts that appear on both sides: the proxy
// writes "pypi.org:443" and a command line says "pypi.org".
func TestRead_PortStrippedAndCaseFolded(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "a", ts: stamp(base, 1), action: "WARN", host: "PyPI.org:443"},
		{seq: 2, requestID: "b", ts: stamp(base, 2), action: "WARN", host: "pypi.org:443"},
	})

	obs := Read(path, Window{Start: base.Add(-time.Minute)})

	if len(obs.Hosts) != 1 {
		t.Fatalf("got %d hosts, want 1 (two spellings of one destination): %+v", len(obs.Hosts), obs.Hosts)
	}
	if obs.Hosts[0].Host != "pypi.org" {
		t.Errorf("host = %q, want \"pypi.org\"", obs.Hosts[0].Host)
	}
	if obs.Hosts[0].Attempts != 2 {
		t.Errorf("attempts = %d, want 2", obs.Hosts[0].Attempts)
	}
	if obs.DistinctHosts != 1 {
		t.Errorf("distinct = %d, want 1", obs.DistinctHosts)
	}
}

// TestRead_AttemptsAndDistinctAreSeparate is the retrying-client case measured
// on the real proxy: the client makes four CONNECTs to one host in three
// seconds. One destination, four attempts. A report printing only the larger
// number makes a retry look like activity.
func TestRead_AttemptsAndDistinctAreSeparate(t *testing.T) {
	var rows []fixtureRow
	for i := 1; i <= 4; i++ {
		rows = append(rows, fixtureRow{
			seq: int64(i), requestID: fmt.Sprintf("req-%d", i),
			ts:     stamp(base.Add(time.Duration(i)*time.Second), float64(i)),
			action: "ALLOW", reason: "under_limit", host: "api.anthropic.com:443",
		})
	}
	obs := Read(newStore(t, rows), Window{Start: base})

	if obs.Attempts != 4 {
		t.Errorf("attempts = %d, want 4", obs.Attempts)
	}
	if obs.DistinctHosts != 1 {
		t.Errorf("distinct = %d, want 1", obs.DistinctHosts)
	}
}

// TestRead_DialFailureFoldsIntoOneAttempt covers the one request that produces
// TWO rows: the chain's verdict, then the dial outcome. Counting both would
// double every unreachable destination.
func TestRead_DialFailureFoldsIntoOneAttempt(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "req-1", ts: stamp(base, 1), action: "ALLOW", reason: "under_limit", host: "down.example:443"},
		{seq: 2, requestID: "req-1", ts: stamp(base, 2), action: "BLOCK_REQUEST", reason: "dial_failed_connection_refused", host: "down.example:443"},
	})

	obs := Read(path, Window{Start: base.Add(-time.Minute)})

	d, ok := find(obs, "down.example")
	if !ok {
		t.Fatalf("host missing: %+v", obs.Hosts)
	}
	if d.Attempts != 1 {
		t.Errorf("attempts = %d, want 1: the verdict row and the dial outcome are one "+
			"request, and counting both doubles every failed dial", d.Attempts)
	}
	if !d.Unreached {
		t.Error("unreached = false, but every row for this host ended in a dial failure. " +
			"\"Allowed but never reached\" and \"reached\" are different facts, and a " +
			"report that conflates them asserts a host was contacted when the " +
			"connection was refused.")
	}
	var sawDial bool
	for _, r := range d.Reasons {
		if r == "dial_failed_connection_refused" {
			sawDial = true
		}
	}
	if !sawDial {
		t.Errorf("reasons = %v, want the dial outcome attached", d.Reasons)
	}
}

// TestRead_ReachedHostIsNotUnreached is the healthy twin of the case above.
func TestRead_ReachedHostIsNotUnreached(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "req-1", ts: stamp(base, 1), action: "WARN",
			reason: "host_not_in_allowlist_observed", host: "pypi.org:443"},
	})
	obs := Read(path, Window{Start: base.Add(-time.Minute)})
	d, _ := find(obs, "pypi.org")
	if d.Unreached {
		t.Error("a host with a verdict row and no dial failure was marked unreached")
	}
}

// TestRead_RowsOutsideTheWindowAreInherited covers the shared-proxy case. Such
// rows are excluded from accounting but still reported: "the proxy saw traffic
// that was not this session's" is a fact the report must be able to state, and
// discarding it silently makes a shared proxy look quiet.
func TestRead_RowsOutsideTheWindowAreInherited(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "old", ts: stamp(base.Add(-time.Hour), 1), action: "WARN", host: "earlier.example:443"},
		{seq: 2, requestID: "mine", ts: stamp(base.Add(time.Second), 2), action: "WARN", host: "pypi.org:443"},
		{seq: 3, requestID: "later", ts: stamp(base.Add(time.Hour), 3), action: "WARN", host: "after.example:443"},
	})

	obs := Read(path, Window{Start: base, End: base.Add(time.Minute)})

	if obs.DistinctHosts != 1 {
		t.Errorf("distinct = %d, want 1: only the in-window host counts", obs.DistinctHosts)
	}
	if obs.Inherited != 2 {
		t.Errorf("inherited = %d, want 2", obs.Inherited)
	}
	for _, name := range []string{"earlier.example", "after.example"} {
		d, ok := find(obs, name)
		if !ok {
			t.Errorf("%s is absent; an out-of-window row is excluded from accounting, not dropped", name)
			continue
		}
		if !d.Inherited {
			t.Errorf("%s is not marked inherited", name)
		}
	}
}

// TestRead_HostInAndOutOfWindowCountsOnlyTheInWindowRows is the case found by
// running the real thing rather than by reading the code.
//
// The same proxy serves session after session, so a popular host — pypi.org,
// registry.npmjs.org — very often has rows from an EARLIER session and rows
// from this one. The destination is rightly not marked inherited, since this
// session did reach it. The trap is the count: folding both sides into
// attempts puts the earlier session's traffic into this session's headline
// number, and a host that appears on both sides is exactly where nobody would
// notice. The first end-to-end run reported "2 distinct, 4 attempts" for two
// hosts reached twice each, and half of those attempts were another session's.
func TestRead_HostInAndOutOfWindowCountsOnlyTheInWindowRows(t *testing.T) {
	path := newStore(t, []fixtureRow{
		// Earlier session, same proxy, same host.
		{seq: 1, requestID: "old-1", ts: stamp(base.Add(-time.Hour), 1), action: "WARN", host: "pypi.org:443"},
		{seq: 2, requestID: "old-2", ts: stamp(base.Add(-time.Hour), 2), action: "WARN", host: "pypi.org:443"},
		// This session.
		{seq: 3, requestID: "mine-1", ts: stamp(base.Add(time.Second), 3), action: "WARN", host: "pypi.org:443"},
	})

	obs := Read(path, Window{Start: base, End: base.Add(time.Minute)})

	d, ok := find(obs, "pypi.org")
	if !ok {
		t.Fatalf("pypi.org missing: %+v", obs.Hosts)
	}
	if d.Inherited {
		t.Error("the destination is marked inherited although this session reached it")
	}
	if d.Attempts != 1 {
		t.Errorf("attempts = %d, want 1: only the in-window row is this session's", d.Attempts)
	}
	if d.InheritedAttempts != 2 {
		t.Errorf("inherited_attempts = %d, want 2", d.InheritedAttempts)
	}
	if obs.Attempts != 1 {
		t.Errorf("total attempts = %d, want 1. Counting the earlier session's rows here "+
			"puts its traffic in this session's headline number.", obs.Attempts)
	}
	if obs.Inherited != 2 {
		t.Errorf("total inherited = %d, want 2", obs.Inherited)
	}
	if obs.DistinctHosts != 1 {
		t.Errorf("distinct = %d, want 1", obs.DistinctHosts)
	}
}

// TestRead_ForeignRunIDIsInherited is the other ground, and it is decisive
// whatever the clock says.
func TestRead_ForeignRunIDIsInherited(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "a", runID: "other-run", ts: stamp(base, 1), action: "WARN", host: "elsewhere.example:443"},
		{seq: 2, requestID: "b", runID: "mine", ts: stamp(base, 2), action: "WARN", host: "pypi.org:443"},
	})

	obs := Read(path, Window{RunID: "mine", Start: base.Add(-time.Minute)})

	if d, _ := find(obs, "elsewhere.example"); !d.Inherited {
		t.Error("a row carrying another run id was not marked inherited")
	}
	if d, _ := find(obs, "pypi.org"); d.Inherited {
		t.Error("this session's own row was marked inherited")
	}
}

// TestRead_UnparseableStampsDeclineTheWindow is the "never claim what was not
// enforced" rule. With no usable clock the rows are still reported and the flag
// says the window was not applied; marking them inherited would silently delete
// real destinations, and claiming the window held would attribute another
// session's traffic to this one.
func TestRead_UnparseableStampsDeclineTheWindow(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "a", ts: "not a timestamp at all", action: "WARN", host: "pypi.org:443"},
	})

	obs := Read(path, Window{Start: base, End: base.Add(time.Minute)})

	if !obs.Observed {
		t.Fatalf("not observed: %s", obs.Reason)
	}
	if obs.WindowApplied {
		t.Error("window_applied is true although no stamp parsed")
	}
	if obs.DistinctHosts != 1 {
		t.Errorf("distinct = %d, want 1: the row must still be reported", obs.DistinctHosts)
	}
}

// TestRead_MissingStoreIsCoverageNotFailure is the degradation contract. A
// report that errored when the proxy was not running would be a report nobody
// could use to learn that the proxy was not running.
func TestRead_MissingStoreIsCoverageNotFailure(t *testing.T) {
	obs := Read(filepath.Join(t.TempDir(), "absent.db"), Window{Start: base})

	if obs.Observed {
		t.Error("observed = true for a store that does not exist")
	}
	if obs.Reason != NotObservedNoStore {
		t.Errorf("reason = %q, want %q", obs.Reason, NotObservedNoStore)
	}
	if len(obs.Hosts) != 0 {
		t.Error("an unreadable store yielded hosts; an empty list is the answer to a " +
			"different question than \"not observed\"")
	}
}

// TestRead_EmptyPathIsNotObserved covers the case where no proxy store was ever
// configured, which must not be reported as "no destinations".
func TestRead_EmptyPathIsNotObserved(t *testing.T) {
	if obs := Read("", Window{}); obs.Observed || obs.Reason != NotObservedNoStore {
		t.Errorf("Read(\"\") = observed %v reason %q", obs.Observed, obs.Reason)
	}
}

// TestRead_WrongSchemaIsNamedNotGuessed keeps a pointing-at-the-wrong-file
// mistake legible instead of rendering it as silence.
func TestRead_WrongSchemaIsNamedNotGuessed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE something_else (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	obs := Read(path, Window{Start: base})
	if obs.Observed {
		t.Error("observed = true against a database with no causal_records table")
	}
	if obs.Reason != NotObservedNoTable {
		t.Errorf("reason = %q, want %q", obs.Reason, NotObservedNoTable)
	}
}

// TestRead_NeverWritesToTheStore matters because the file it reads is an audit
// store the operator may need as evidence. The connection is opened mode=ro; a
// reader bug must not be able to change it.
func TestRead_NeverWritesToTheStore(t *testing.T) {
	path := newStore(t, []fixtureRow{
		{seq: 1, requestID: "a", ts: stamp(base, 1), action: "WARN", host: "pypi.org:443"},
	})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if obs := Read(path, Window{Start: base.Add(-time.Minute)}); !obs.Observed {
		t.Fatalf("not observed: %s", obs.Reason)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the store's bytes changed; the reader must open it read-only")
	}
}

// TestRead_Deterministic is the report's byte-identical-render property at this
// layer. Map iteration order is not stable, so the host list is sorted.
func TestRead_Deterministic(t *testing.T) {
	rows := []fixtureRow{
		{seq: 1, requestID: "a", ts: stamp(base, 1), action: "WARN", host: "zeta.example:443"},
		{seq: 2, requestID: "b", ts: stamp(base, 2), action: "WARN", host: "alpha.example:443"},
		{seq: 3, requestID: "c", ts: stamp(base, 3), action: "WARN", host: "mid.example:443"},
	}
	path := newStore(t, rows)
	w := Window{Start: base.Add(-time.Minute)}

	first := Read(path, w)
	for i := 0; i < 5; i++ {
		again := Read(path, w)
		if len(again.Hosts) != len(first.Hosts) {
			t.Fatalf("host count moved between renders")
		}
		for j := range first.Hosts {
			if first.Hosts[j].Host != again.Hosts[j].Host {
				t.Fatalf("order moved: %v vs %v", first.Hosts, again.Hosts)
			}
		}
	}
	if first.Hosts[0].Host != "alpha.example" {
		t.Errorf("hosts are not sorted: %v", first.Hosts)
	}
}

// TestSummarise_ARunWithNoWindowIsNotObservedRatherThanEverything is the
// phantom-session defect an Opus review of this branch found.
//
// WindowApplied carried TWO distinct degradations on one flag: the store's
// timestamps could not be parsed, and the RUN recorded no window at all. The
// second makes isInherited return false for every row, so the whole proxy
// database counts as this session's traffic -- with no declarations to match,
// every host becomes "reached but never named" and proxy-on-path becomes true.
//
// It is reachable through the shipped path: report deliberately renders evicted
// run directories, and an evicted run has no coverage records. After any
// size-cap eviction, the report grew a phantom session whose accusation list
// was the entire store.
//
// And the only explanation offered was the wrong one: the text said "the
// proxy's timestamps could not be parsed" when they had parsed perfectly. Two
// causes, one message, and the message named the cause that was not true.
func TestSummarise_ARunWithNoWindowIsNotObservedRatherThanEverything(t *testing.T) {
	// Rows whose timestamps parse fine, and a run that recorded no window.
	rows := []row{
		{seq: 1, requestID: "r1", host: "someone-elses.example", when: base, whenOK: true},
		{seq: 2, requestID: "r2", host: "another.example", when: base, whenOK: true},
	}

	obs := summarise(rows, Window{}, "/tmp/causal.db")

	if obs.Observed {
		t.Error("a run with no recorded window reports its destinations as observed, so " +
			"the whole store's history is attributed to it: with no declarations every " +
			"host becomes \"reached but never named\"")
	}
	if obs.Reason != NotObservedNoWindow {
		t.Errorf("reason = %q, want %q; the timestamps parsed, so blaming them names a "+
			"cause that is not true", obs.Reason, NotObservedNoWindow)
	}
	if len(obs.Hosts) != 0 {
		t.Errorf("hosts = %v, want none: these rows belong to whichever sessions did "+
			"record a window", obs.Hosts)
	}
}

// TestSummarise_UnparseableTimestampsAreStillReportedWithTheWindowFlag is the
// other half, and the reason the two must not share a code. Here the run DOES
// have a window and the clock is unusable, so reporting every row with the flag
// set is right -- the rows exist and the reader is told the window could not be
// enforced.
func TestSummarise_UnparseableTimestampsAreStillReportedWithTheWindowFlag(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "r1", host: "pypi.org", whenOK: false},
	}

	obs := summarise(rows, Window{Start: base}, "/tmp/causal.db")

	if !obs.Observed {
		t.Fatal("rows with unparseable timestamps are dropped entirely; the destinations " +
			"were seen and only their instants are unknown")
	}
	if obs.WindowApplied {
		t.Error("window_applied is true with no usable clock")
	}
	if len(obs.Hosts) != 1 {
		t.Errorf("hosts = %v, want the one row reported", obs.Hosts)
	}
}

// TestSummarise_ADialFailureWithNoRequestIDStillMarksTheHostUnreached.
//
// Dial outcomes are folded onto the verdict row by request_id. A dial_failed
// row carrying no request id fell through that branch, became its own attempt
// keyed by sequence number, and then missed the outcome lookup -- so Unreached
// stayed false and the report asserted the host WAS contacted when the
// connection had been refused. "Allowed but never reached" and "reached" are
// the two facts this field exists to separate.
//
// The reader already has an explicit branch for an empty request id two lines
// below, so it evidently considers one possible; whether the proxy emits such a
// row is a question about the closed product's writer. Handling it costs one
// fallback keyed on the host, and not handling it is wrong in the direction
// that overstates what was observed.
func TestSummarise_ADialFailureWithNoRequestIDStillMarksTheHostUnreached(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "", host: "unreachable.example", action: "ALLOW",
			reason: "under_limit", when: base, whenOK: true},
		{seq: 2, requestID: "", host: "unreachable.example",
			reason: dialFailedPrefix + "timeout", when: base, whenOK: true},
	}

	obs := summarise(rows, Window{Start: base.Add(-time.Minute)}, "/tmp/causal.db")

	if len(obs.Hosts) != 1 {
		t.Fatalf("hosts = %v, want one destination", obs.Hosts)
	}
	if !obs.Hosts[0].Unreached {
		t.Error("a host whose dial failed is reported as reached, because the outcome " +
			"row carried no request id to fold it onto")
	}
}

// TestSummarise_TimestampsExcludeInheritedRows. The Destination doc is explicit
// that in-window and inherited counts are kept apart so another session's
// traffic cannot enter this session's numbers -- and then first_seen/last_seen
// were computed over every row, so a consumer reading them got another
// session's instant. Not rendered in the text form; JSON carries them.
func TestSummarise_TimestampsExcludeInheritedRows(t *testing.T) {
	windowStart := base
	rows := []row{
		// Well before the window: another session's traffic.
		{seq: 1, requestID: "old", host: "pypi.org", action: "ALLOW",
			when: base.Add(-time.Hour), whenOK: true},
		// This session's.
		{seq: 2, requestID: "mine", host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{Start: windowStart}, "/tmp/causal.db")

	if len(obs.Hosts) != 1 {
		t.Fatalf("hosts = %v, want one", obs.Hosts)
	}
	d := obs.Hosts[0]
	if d.InheritedAttempts != 1 || d.Attempts != 1 {
		t.Fatalf("premise: attempts %d in window, %d inherited, want 1 and 1",
			d.Attempts, d.InheritedAttempts)
	}
	got, err := time.Parse(time.RFC3339, d.FirstSeen)
	if err != nil {
		t.Fatalf("first_seen %q does not parse: %v", d.FirstSeen, err)
	}
	if got.Before(windowStart) {
		t.Errorf("first_seen = %s, which is before this session's window opened at %s: "+
			"it is another session's instant, and the counts are kept apart precisely "+
			"so that cannot happen", d.FirstSeen, windowStart.UTC().Format(time.RFC3339))
	}
}
