package acceptance

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestH15_Permissions asserts the store is readable only by its owner. The
// records name every tool a session invoked and the path of its transcript,
// which together describe what someone was working on.
func TestH15_Permissions(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	want := map[string]fs.FileMode{
		".":                                0o700,
		"runs":                             0o700,
		"install.key":                      0o600,
		"install.json":                     0o600,
		filepath.Join("runs", testSession): 0o700,
		filepath.Join("runs", testSession, "records.ndjson"):  0o600,
		filepath.Join("runs", testSession, "coverage.ndjson"): 0o600,
		filepath.Join("runs", testSession, "seq"):             0o600,
		filepath.Join("runs", testSession, "probe"):           0o600,
	}
	got := walkStore(t, e.home)
	for rel, mode := range want {
		entry, ok := got[rel]
		if !ok {
			t.Errorf("%s is missing from the store", rel)
			continue
		}
		if entry.mode.Perm() != mode {
			t.Errorf("%s has mode %04o, want %04o", rel, entry.mode.Perm(), mode)
		}
	}
}

// TestH15_NDJSONFraming asserts the framing a downstream reader depends on.
func TestH15_NDJSONFraming(t *testing.T) {
	e := newEnv(t)
	e.declarationsAfter("ls -la", `echo "with a \" quote and a newline\n inside"`, "git log --oneline")

	for _, name := range []string{"records.ndjson", "coverage.ndjson"} {
		body, err := os.ReadFile(filepath.Join(e.home, "runs", testSession, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if len(body) == 0 {
			t.Errorf("%s is empty", name)
			continue
		}
		if body[len(body)-1] != '\n' {
			t.Errorf("%s does not end with a newline; a reader cannot tell a truncated last record from a complete one", name)
		}
		for i, line := range bytes.Split(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				t.Errorf("%s line %d is blank", name, i+1)
			}
		}
	}
}

// TestH15_SchemaVersionOnEveryRecord is what lets a reader skip a record it does
// not understand instead of guessing at it.
func TestH15_SchemaVersionOnEveryRecord(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.declarationsAfter("ls", "pwd")
	e.probe("end", testSession)
	e.forget("1h")

	var all []record
	all = append(all, e.records(testSession)...)
	all = append(all, e.read(testSession, "coverage.ndjson")...)
	all = append(all, e.gaps()...)
	if len(all) == 0 {
		t.Fatal("no records were written")
	}
	// Asserted against the writer's own constant rather than a literal. The
	// invariant is "every record carries the version the writer was at", not
	// "every record says 1", and a literal here is a line to chase on every
	// schema bump -- which is how a bump ends up landing with some records
	// carrying the new version and some the old.
	want := float64(store.SchemaVersion)
	for i, r := range all {
		if r.fields["schema_version"] != want {
			t.Errorf("record %d (type %q) has schema_version %v, want %v", i, r.typ(), r.fields["schema_version"], want)
		}
		if r.typ() == "" {
			t.Errorf("record %d has no type discriminator", i)
		}
	}
}

// TestH15_ForgetLeavesAGap: records leave the store only with a marker saying
// they did.
func TestH15_ForgetLeavesAGap(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.declarationsAfter("ls", "pwd", "git status")

	res := e.forget("1h")
	if res.exitCode != 0 {
		t.Fatalf("forget: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	if got := e.declarations(testSession); len(got) != 0 {
		t.Errorf("%d declarations survived forget --since 1h, want 0", len(got))
	}
	gaps := e.gaps()
	if len(gaps) != 1 {
		t.Fatalf("got %d gap records, want exactly 1", len(gaps))
	}
	g := gaps[0]
	assertKeySet(t, g, gapKeys)
	if g.str("reason") != "forget" || g.str("session_id") != testSession {
		t.Errorf("gap is %v, want reason forget for %s", g.fields, testSession)
	}
	// Three declarations and three terminals left.
	if g.fields["removed_records"] != float64(6) {
		t.Errorf("gap says %v records removed, want 6", g.fields["removed_records"])
	}

	rep := e.report(testSession)
	if len(rep.Gaps) != 1 || !e.hasReason(rep, "gap") {
		t.Errorf("report does not surface the gap: gaps=%d reasons=%v", len(rep.Gaps), rep.Coverage.Reasons)
	}
	// Coverage records are not what forget forgets; the run's history of
	// whether it could be trusted stays.
	if len(e.coverage(testSession, "call")) != 3 {
		t.Errorf("forget removed coverage records; it must only remove declarations and terminals")
	}
}

// TestH15_ForgetIsBounded: --since removes what is at or after the instant and
// nothing before it.
func TestH15_ForgetIsBounded(t *testing.T) {
	e := newEnv(t)
	// Distinct ids, as a real session has: forget removes a declaration and
	// its terminal as a pair, keyed by tool_use_id.
	call := func(id, command string) {
		p := defaultPayload()
		p.ToolUseID = id
		p.ToolInput = map[string]any{"command": command}
		e.mustHook(p.build(t))
	}
	call("toolu_before", "first")
	cut := time.Now().UTC().Add(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	call("toolu_after_1", "second")
	call("toolu_after_2", "third")

	res := e.forget(cut.Format(time.RFC3339Nano))
	if res.exitCode != 0 {
		t.Fatalf("forget: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := e.declarations(testSession); len(got) != 1 {
		t.Errorf("%d declarations survived, want the 1 recorded before the cut", len(got))
	}
}

// TestH15_ForgetBeforeLeavesAGap is the retention counterpart of the item
// above: --since forgets what just happened, --before forgets what is old, and
// both leave the same marker. A window with its lower end open has no bound to
// name, so the gap's own span starts at the earliest record it removed.
func TestH15_ForgetBeforeLeavesAGap(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.declarationsAfter("ls", "pwd", "git status")
	first := e.declarations(testSession)[0].fields["recorded_at_unix_ms"]

	cut := time.Now().UTC().Add(time.Hour)
	res := e.forgetBefore(cut.Format(time.RFC3339Nano))
	if res.exitCode != 0 {
		t.Fatalf("forget --before: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	if got := e.declarations(testSession); len(got) != 0 {
		t.Errorf("%d declarations survived forget --before, want 0", len(got))
	}
	gaps := e.gaps()
	if len(gaps) != 1 {
		t.Fatalf("got %d gap records, want exactly 1", len(gaps))
	}
	g := gaps[0]
	assertKeySet(t, g, gapKeys)
	if g.str("reason") != "forget" || g.str("session_id") != testSession {
		t.Errorf("gap is %v, want reason forget for %s", g.fields, testSession)
	}
	if g.fields["removed_records"] != float64(6) {
		t.Errorf("gap says %v records removed, want 6", g.fields["removed_records"])
	}
	if g.fields["from_unix_ms"] != first {
		t.Errorf("gap starts at %v, want %v: with no lower bound named, the span starts at the earliest record removed",
			g.fields["from_unix_ms"], first)
	}
	if g.fields["to_unix_ms"] != float64(cut.UnixMilli()) {
		t.Errorf("gap ends at %v, want %v: the bound the caller named", g.fields["to_unix_ms"], cut.UnixMilli())
	}

	rep := e.report(testSession)
	if len(rep.Gaps) != 1 || !e.hasReason(rep, "gap") {
		t.Errorf("report does not surface the gap: gaps=%d reasons=%v", len(rep.Gaps), rep.Coverage.Reasons)
	}
	if len(e.coverage(testSession, "call")) != 3 {
		t.Errorf("forget --before removed coverage records; it must only remove declarations, executions and terminals")
	}
}

// TestH15_ForgetBeforeIsBounded is TestH15_ForgetIsBounded pointed the other
// way: --before removes what is strictly before the instant and nothing at or
// after it.
func TestH15_ForgetBeforeIsBounded(t *testing.T) {
	e := newEnv(t)
	call := func(id, command string) {
		p := defaultPayload()
		p.ToolUseID = id
		p.ToolInput = map[string]any{"command": command}
		e.mustHook(p.build(t))
	}
	call("toolu_before_1", "first")
	call("toolu_before_2", "second")
	cut := time.Now().UTC().Add(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	call("toolu_after", "third")

	res := e.forgetBefore(cut.Format(time.RFC3339Nano))
	if res.exitCode != 0 {
		t.Fatalf("forget --before: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("%d declarations survived, want the 1 recorded at or after the cut", len(decls))
	}
	if got := decls[0].str("tool_use_id"); got != "toolu_after" {
		t.Errorf("the surviving declaration is %q, want toolu_after", got)
	}
}

// TestH15_ForgetRefusesBothWindows: the two flags name opposite ends of one
// window, and a command given both has been asked for two different things.
// Resolving that silently would remove whichever set the implementation
// happened to prefer.
func TestH15_ForgetRefusesBothWindows(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.declarationsAfter("ls", "pwd")

	res := e.run("", nil, "forget", "--since", "1h", "--before", "1h")
	if res.exitCode != 1 {
		t.Fatalf("forget --since --before: exit %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "--since") || !strings.Contains(res.stderr, "--before") {
		t.Errorf("the refusal does not name both flags: %q", res.stderr)
	}
	if got := len(e.declarations(testSession)); got != 2 {
		t.Errorf("%d declarations survived a refused forget, want 2", got)
	}
	if got := len(e.gaps()); got != 0 {
		t.Errorf("a refused forget wrote %d gap records", got)
	}
}

// TestH15_ForgetTakesTheExecutionWithThePair: executions arrived after forget
// did, and a plan built from declarations and terminals alone would leave an
// execution record for a call whose declaration is gone -- a call that reads as
// having run without ever having been asked for.
func TestH15_ForgetTakesTheExecutionWithThePair(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	// Re-stamp the set around the cut: the execution before it, the
	// declaration and its terminal after. Only the execution is in the window,
	// and all three have to leave together.
	cut := time.Now().UTC().Truncate(time.Second)
	restamp(t, e, map[string]time.Time{
		"declaration": cut.Add(time.Second),
		"terminal":    cut.Add(time.Second),
		"execution":   cut.Add(-time.Second),
	})

	if res := e.forgetBefore(cut.Format(time.RFC3339)); res.exitCode != 0 {
		t.Fatalf("forget --before: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := e.records(testSession); len(got) != 0 {
		t.Errorf("%d records survived; the declaration, its execution and its terminal leave together: %v", len(got), got)
	}
	gaps := e.gaps()
	if len(gaps) != 1 || gaps[0].fields["removed_records"] != float64(3) {
		t.Errorf("gaps are %v, want one gap removing 3 records", gaps)
	}
}

// restamp rewrites records.ndjson with one recorded-at instant per record type,
// which is how a test puts a set of records on chosen sides of a cut.
func restamp(t *testing.T, e *env, at map[string]time.Time) {
	t.Helper()
	path := filepath.Join(e.home, "runs", testSession, "records.ndjson")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		typ, _ := rec["type"].(string)
		when, ok := at[typ]
		if !ok {
			t.Fatalf("premise broken: no instant given for a %q record", typ)
		}
		rec["recorded_at_unix_ms"] = when.UnixMilli()
		out, _ := json.Marshal(rec)
		lines = append(lines, string(out))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestH15_SizeCapEvictionLeavesAGap: the other way records leave.
func TestH15_SizeCapEvictionLeavesAGap(t *testing.T) {
	e := newEnv(t)
	e.watched("old-session")
	p := defaultPayload()
	p.SessionID = "old-session"
	e.mustHook(p.build(t))
	x := defaultPost()
	x.SessionID = "old-session"
	e.mustPost(x.build(t))
	e.probe("end", "old-session")

	// Age the old run past the grace period, then bring a new session up
	// under a cap nothing fits in.
	old := time.Now().Add(-3 * time.Hour)
	oldDir := filepath.Join(e.home, "runs", "old-session")
	entries, _ := os.ReadDir(oldDir)
	for _, f := range entries {
		if err := os.Chtimes(filepath.Join(oldDir, f.Name()), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if res := e.probe("start", "new-session", "RASHOMON_STORE_CAP_BYTES=1"); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d", res.exitCode)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old run was not evicted under a 1-byte cap")
	}
	if _, err := os.Stat(filepath.Join(e.home, "runs", "new-session")); err != nil {
		t.Errorf("the session that triggered eviction was itself evicted")
	}
	gaps := e.gaps()
	if len(gaps) != 1 {
		t.Fatalf("got %d gap records, want exactly 1", len(gaps))
	}
	if gaps[0].str("reason") != "size_cap" || gaps[0].str("session_id") != "old-session" {
		t.Errorf("gap is %v, want reason size_cap for old-session", gaps[0].fields)
	}
	// A declaration, its terminal and its execution: the count covers every
	// record type the run held, not the two that existed when it was written.
	if gaps[0].fields["removed_records"] != float64(3) {
		t.Errorf("gap says %v records removed, want 3", gaps[0].fields["removed_records"])
	}
}

// TestH15_LiveRunsAreNotEvicted: a run written to recently may still be in use.
func TestH15_LiveRunsAreNotEvicted(t *testing.T) {
	e := newEnv(t)
	p := defaultPayload()
	p.SessionID = "recent"
	e.mustHook(p.build(t))
	e.probe("start", "another", "RASHOMON_STORE_CAP_BYTES=1")

	if _, err := os.Stat(filepath.Join(e.home, "runs", "recent")); err != nil {
		t.Errorf("a run written seconds ago was evicted")
	}
	if len(e.gaps()) != 0 {
		t.Errorf("a gap was written for a run that must not be evicted")
	}
}

// TestH15_SessionIDCannotEscapeTheStore covers a value that arrives from outside
// the process and is then used to build a path. Joined unchecked, a session id
// of ../../.. writes wherever it likes.
func TestH15_SessionIDCannotEscapeTheStore(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t)
	e.home = filepath.Join(root, "store")

	p := defaultPayload()
	p.SessionID = "../../escaped"
	e.mustHook(p.build(t))

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range entries {
		if en.Name() != "store" {
			t.Errorf("%s was created outside the store root", en.Name())
		}
	}
	runs, err := os.ReadDir(filepath.Join(e.home, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d run directories, want 1", len(runs))
	}
	if name := runs[0].Name(); name == "../../escaped" || name == "escaped" {
		t.Errorf("run directory is named %q, which is the raw session id", name)
	}
}

// TestH15_ForgetRemovesPairsTogether: a declaration and its terminal that
// straddle the cut leave together. Removing only the terminal would leave a
// declaration that reads exactly like a killed handler.
func TestH15_ForgetRemovesPairsTogether(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	// Re-stamp the pair around the cut: declaration a second before it, its
	// terminal a second after.
	cut := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(e.home, "runs", testSession, "records.ndjson")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		switch rec["type"] {
		case "declaration":
			rec["recorded_at_unix_ms"] = cut.Add(-time.Second).UnixMilli()
		case "terminal":
			rec["recorded_at_unix_ms"] = cut.Add(time.Second).UnixMilli()
		}
		out, _ := json.Marshal(rec)
		lines = append(lines, string(out))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if res := e.forget(cut.Format(time.RFC3339)); res.exitCode != 0 {
		t.Fatalf("forget: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := e.records(testSession); len(got) != 0 {
		t.Errorf("%d records survived; the pair must leave together", len(got))
	}
	gaps := e.gaps()
	if len(gaps) != 1 || gaps[0].fields["removed_records"] != float64(2) {
		t.Errorf("gaps are %v, want one gap removing 2 records", gaps)
	}
	rep := e.report(testSession)
	if len(rep.Declarations.Unterminated) != 0 {
		t.Errorf("forget left a declaration reading as unterminated: %v", rep.Declarations.Unterminated)
	}
}

// TestH15_EvictedRunsStillReportTheirGap: a run eviction removed has no
// directory, and a report that could not show its gap would be hiding a
// deletion.
func TestH15_EvictedRunsStillReportTheirGap(t *testing.T) {
	e := newEnv(t)
	p := defaultPayload()
	p.SessionID = "old-session"
	e.mustHook(p.build(t))
	old := time.Now().Add(-3 * time.Hour)
	oldDir := filepath.Join(e.home, "runs", "old-session")
	entries, _ := os.ReadDir(oldDir)
	for _, f := range entries {
		_ = os.Chtimes(filepath.Join(oldDir, f.Name()), old, old)
	}
	e.probe("start", "new-session", "RASHOMON_STORE_CAP_BYTES=1")
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("premise broken: old run was not evicted")
	}

	var found *reportSession
	for _, s := range e.reportAll() {
		if s.SessionID == "old-session" {
			s := s
			found = &s
		}
	}
	if found == nil {
		t.Fatal("the all-sessions report does not mention the evicted run")
	}
	if len(found.Gaps) != 1 || !e.hasReason(*found, "gap") || found.Coverage.State != "unverified" {
		t.Errorf("evicted run renders as %s with %d gaps (%v)", found.Coverage.State, len(found.Gaps), found.Coverage.Reasons)
	}
	one := e.report("old-session")
	if len(one.Gaps) != 1 {
		t.Errorf("--session on an evicted run renders %d gaps, want 1", len(one.Gaps))
	}
}
