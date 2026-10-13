package acceptance

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// overlongLine is longer than the store's reader limit (16 MiB, maxLine in
// internal/store/lines.go): the line a record could become before the writer
// was bounded, a 2.9 MB run of `<` escaping six-fold.
var overlongLine = strings.Repeat("x", 16<<20+1)

// ageRun moves every file of a run past eviction's one-hour grace period.
func (e *env) ageRun(sessionID string, by time.Duration) {
	e.t.Helper()
	dir := filepath.Join(e.home, "runs", sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.t.Fatal(err)
	}
	old := time.Now().Add(-by)
	for _, f := range entries {
		if err := os.Chtimes(filepath.Join(dir, f.Name()), old, old); err != nil {
			e.t.Fatal(err)
		}
	}
}

// TestREC32c_OverlongLineNeitherHidesTheRunNorStopsEviction is the rubric's
// lld-3 scenario end to end: a run holding one line over the reader's limit.
// report used to exit 1 on it; the end probe could not read the run; and once
// it was the oldest run, eviction stopped for good while the probe threw the
// error away. Now report renders the run with the line counted as unreadable,
// and eviction removes it, saying the count it removed is unknown.
func TestREC32c_OverlongLineNeitherHidesTheRunNorStopsEviction(t *testing.T) {
	e := newEnv(t)
	e.watched("old-session")
	p := defaultPayload()
	p.SessionID = "old-session"
	e.mustHook(p.build(t))
	appendRawLine(t, e.recordsPath("old-session"), overlongLine)
	e.probe("end", "old-session")

	rep := e.report("old-session")
	if rep.SkippedRecords != 1 || rep.Declarations.Recorded != 1 {
		t.Errorf("report read %d declarations with %d skipped, want 1 and 1", rep.Declarations.Recorded, rep.SkippedRecords)
	}
	if !e.hasReason(rep, "records_unreadable") {
		t.Errorf("reasons %v do not name records_unreadable", rep.Coverage.Reasons)
	}

	e.ageRun("old-session", 3*time.Hour)
	res := e.probe("start", "new-session", "RASHOMON_STORE_CAP_BYTES=1")
	if res.exitCode != 0 || res.stdout != "" {
		t.Fatalf("probe start: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	if _, err := os.Stat(filepath.Join(e.home, "runs", "old-session")); !os.IsNotExist(err) {
		t.Fatalf("the run with an over-long line was not evicted under a 1-byte cap")
	}
	gaps := e.gaps()
	if len(gaps) != 1 || gaps[0].fields["removed_records"] != float64(-1) {
		t.Fatalf("gaps %v, want one size_cap gap counting -1 (unknown) records", gaps)
	}
	text := e.run("", nil, "report", "--session", "old-session")
	if !strings.Contains(text.stdout, "size_cap: an unknown number of records removed") {
		t.Errorf("the report does not say the count is unknown:\n%s", text.stdout)
	}
}

// TestREC32d_ProbeSaysWhenEvictionFails: the probe threw eviction's error
// away, so a store that could not evict grew past its cap with nothing
// saying so anywhere. It now prints one fixed line on stderr, which reaches
// Claude Code's debug log, and still exits 0 with nothing on stdout.
func TestREC32d_ProbeSaysWhenEvictionFails(t *testing.T) {
	e := newEnv(t)
	e.watched("old-session")
	p := defaultPayload()
	p.SessionID = "old-session"
	e.mustHook(p.build(t))
	e.ageRun("old-session", 3*time.Hour)
	// A gaps file eviction cannot open: a directory in its place.
	if err := os.Remove(filepath.Join(e.home, "gaps.ndjson")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(e.home, "gaps.ndjson"), 0o700); err != nil {
		t.Fatal(err)
	}

	res := e.probe("start", "new-session", "RASHOMON_STORE_CAP_BYTES=1")
	if res.exitCode != 0 || res.stdout != "" {
		t.Fatalf("probe start: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	if strings.TrimSpace(res.stderr) != "rashomon: store eviction incomplete" {
		t.Errorf("probe stderr %q, want the fixed eviction line", res.stderr)
	}
	if _, err := os.Stat(filepath.Join(e.home, "runs", "old-session")); err != nil {
		t.Errorf("eviction removed a run it could not write a gap for: %v", err)
	}
}

// TestREC32e_ForgetSaysWhatItDidAndWhatItCouldNot: forget --host goes on past
// a run it cannot rewrite. It prints what it did remove and still exits 1,
// with the run it left named on stderr -- never "forgot 0" when it forgot
// something, and never success when a run still holds the host.
func TestREC32e_ForgetSaysWhatItDidAndWhatItCouldNot(t *testing.T) {
	e := newEnv(t)
	for _, s := range []string{"sess-damaged", "sess-fine"} {
		e.watched(s)
		p := defaultPayload()
		p.SessionID = s
		p.ToolUseID = "toolu_" + s
		p.ToolInput = map[string]any{"command": "curl https://acme-secret.internal/x"}
		e.mustHook(p.build(t))
	}
	appendRawLine(t, e.recordsPath("sess-damaged"), overlongLine)

	res := e.run("", nil, "forget", "--host", "acme-secret.internal")
	if res.exitCode != 1 {
		t.Errorf("forget --host: exit %d, want 1 while a run still holds the host", res.exitCode)
	}
	if !strings.Contains(res.stdout, "forgot 2 records") {
		t.Errorf("stdout %q does not say what was forgotten", res.stdout)
	}
	if !strings.Contains(res.stderr, "sess-damaged") {
		t.Errorf("stderr %q does not name the run that was left", res.stderr)
	}
	if got := e.declarations("sess-fine"); len(got) != 0 {
		t.Errorf("the readable run still holds %d declarations", len(got))
	}
}

// TestREC32f_ForgetHostThatForgotNothingSaysNothing: forget --host goes on
// past a run it cannot rewrite and prints what it did (REC32e), but a failure
// before any run was touched -- here the store-level forget record, whose
// file cannot be opened -- forgot nothing and suppressed nothing. Its line,
// "later reports still leave it out", would be false, so none is printed.
func TestREC32f_ForgetHostThatForgotNothingSaysNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a file mode does not stop this process here")
	}
	e := newEnv(t)
	seedHostSession(t, e)
	if err := os.WriteFile(filepath.Join(e.home, store.FileForgottenHosts), nil, 0); err != nil {
		t.Fatal(err)
	}

	res := e.run("", nil, "forget", "--host", forgettable)
	if res.exitCode == 0 {
		t.Errorf("forget --host succeeded with its record unwritable:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, "forgot") {
		t.Errorf("forget --host claimed a forget that did not happen: stdout %q", res.stdout)
	}
	if got := len(e.declarations(testSession)); got != 2 {
		t.Errorf("%d declarations left, want 2: the command stops before any run", got)
	}
}

// TestREC32_TornTailNeverSwallowsTheNextRecord: a hook killed in the middle
// of an append leaves a last line with no newline. The next hook's
// declaration used to be glued onto it, so both read as one damaged line and
// the healthy call was lost -- and, its terminal surviving, blamed on the
// lock. The writer now starts a line of its own when the file does not end
// in one: the fragment counts once as unreadable, and the call after it
// reads intact.
func TestREC32_TornTailNeverSwallowsTheNextRecord(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	appendRawBytes(t, e.recordsPath(testSession), `{"type":"declaration","schema_version":4,"tool_use_id":"toolu_cut`)
	next := defaultPayload()
	next.ToolUseID = "toolu_next"
	e.mustHook(next.build(t))
	e.probe("end", testSession)

	rep := e.report(testSession)
	if rep.Declarations.Recorded != 2 || rep.SkippedRecords != 1 {
		t.Errorf("read %d declarations with %d skipped, want 2 and 1", rep.Declarations.Recorded, rep.SkippedRecords)
	}
	if e.hasReason(rep, "lock_timeout") {
		t.Errorf("a torn line is blamed on the lock: %v", rep.Coverage.Reasons)
	}
}

// TestRubricERRORS1_AFailedAppendLeavesNoPartLine: an append whose write
// fails is cut back to where the file ended, so a short write leaves no
// fragment for the next record to land behind, and a record the hook was
// told failed is not there to be read as if it had landed. The fault point
// reports a failure after a whole line went out, which is the case a
// missing cut-back would leave visible.
func TestRubricERRORS1_AFailedAppendLeavesNoPartLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	failed := defaultPayload()
	failed.ToolUseID = "toolu_failed"
	if res := e.hook(failed.build(t), "RASHOMON_FAULT=store.append.write:fail=1"); res.exitCode != 0 {
		t.Fatalf("hook: exit %d", res.exitCode)
	}
	e.mustHook(defaultPayload().build(t))

	decls := e.declarations(testSession)
	if len(decls) != 1 || decls[0].str("tool_use_id") != testToolUseID {
		t.Errorf("declarations %v, want only the one whose write succeeded", decls)
	}
	if got := e.coverage(testSession, "call"); len(got) != 2 || got[0].str("reason") != "internal_error" {
		t.Errorf("call coverage %v, want internal_error for the failed append", got)
	}
}
