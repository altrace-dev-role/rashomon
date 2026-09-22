package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

// H-102 -- a store this binary cannot fully read never renders verified.
//
// internal/store/read.go's classify counts a line it could not parse into
// run.Skipped rather than failing the whole read: an unrecognised
// schema_version, and a line that fails to unmarshal outright, are both
// per-record skips, not fatal errors, so that one bad line cannot hide an
// entire run. That policy is correct and stays.
//
// What was missing is downstream of it. report.go carried SkippedRecords onto
// the session and text.go rendered it as a bare number, and nothing made a
// non-zero count drive a coverage reason. A store holding records this binary
// could not read was therefore free to render `coverage: verified` --
// exactly the violation README's two rules forbid: never print "nothing
// happened" where the truth is "not watching."
//
// Break: revert report.go's build() to stop adding ReasonRecordsUnreadable
// when SkippedRecords > 0. With that reverted, SkippedRecords is a rendered
// number driving no reason, which was the behaviour on main.
//
// ReasonRecordsUnreadable lives in report.Reasons(), not store.Reasons(): it
// is derived from run.Skipped while reading the whole run, and no coverage
// record a hook writes ever carries it, since a hook has no way to know that
// some OTHER line in the file failed to parse.
func TestH102_UnrecognisedSchemaVersionBreaksVerified(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// A good record, through the real hook path, so this run would render
	// verified on its own.
	e.mustHook(defaultPayload().build(t))
	e.probe("end", testSession)

	rep := e.report(testSession)
	if rep.Coverage.State != "verified" || rep.SkippedRecords != 0 {
		t.Fatalf("premise broken: the healthy run is %s with %d skipped, want verified with 0",
			rep.Coverage.State, rep.SkippedRecords)
	}

	// Alongside the good record, one this reader's Accepts() does not
	// recognise -- the shape a store written partly by a newer or older
	// build, or edited by hand, takes.
	appendRawLine(t, e.recordsPath(testSession),
		`{"type":"declaration","schema_version":99,"tool_use_id":"toolu_future","session_id":"`+testSession+`"}`)

	rep = e.report(testSession)
	if rep.SkippedRecords == 0 {
		t.Fatalf("premise broken: a record at schema_version 99 did not bump skipped records")
	}
	if rep.Coverage.State == "verified" {
		t.Errorf("coverage is verified with %d record(s) this binary could not read", rep.SkippedRecords)
	}
	if !e.hasReason(rep, "records_unreadable") {
		t.Errorf("coverage reasons %v do not name records_unreadable", rep.Coverage.Reasons)
	}
}

// TestH102_UnmarshalFailureReachesTheSameReason is the other way a line never
// becomes a record: not a version this reader declines, but bytes that are
// not JSON at all. classify bumps run.Skipped on that path too (the
// json.Unmarshal error case, ahead of the Accepts check), so it must reach the
// same reason rather than passing silently because the version happened to
// look fine.
func TestH102_UnmarshalFailureReachesTheSameReason(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	e.mustHook(defaultPayload().build(t))
	e.probe("end", testSession)

	appendRawLine(t, e.recordsPath(testSession), `this line is not JSON at all`)

	rep := e.report(testSession)
	if rep.SkippedRecords == 0 {
		t.Fatalf("premise broken: a syntactically broken line did not bump skipped records")
	}
	if rep.Coverage.State == "verified" {
		t.Errorf("coverage is verified with %d record(s) this binary could not read", rep.SkippedRecords)
	}
	if !e.hasReason(rep, "records_unreadable") {
		t.Errorf("coverage reasons %v do not name records_unreadable", rep.Coverage.Reasons)
	}
}

// recordsPath is where a run's ordered stream lives on disk, for tests that
// inject a line no writer in this program would produce.
func (e *env) recordsPath(sessionID string) string {
	return filepath.Join(e.home, "runs", sessionID, "records.ndjson")
}

// appendRawLine appends one line verbatim to an NDJSON file the store owns.
// Used to simulate a record this reader cannot parse: an unknown
// schema_version, or a line that is not JSON at all.
func appendRawLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close() //nolint:errcheck // test fixture, write error below is what matters
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("appending to %s: %v", path, err)
	}
}
