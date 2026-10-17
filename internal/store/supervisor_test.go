package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSTR6b_AStoodDownStoreWritesNothing: once the stand-down question
// answers yes -- a supervised child whose parent has gone, or whose hard stop
// has passed -- every append and the probe marker are refused with
// ErrStoodDown before anything is touched: no line, no run directory, no seq
// file. The question is asked again before each one, so a child that was
// healthy for its first append and orphaned before its second writes exactly
// the first (STR-6, step 7: stand down before any new append, never in the
// middle of one).
func TestSTR6b_AStoodDownStoreWritesNothing(t *testing.T) {
	const session = "sess-stood-down"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gone := false
	asked := 0
	st.StandDownWhen(func() bool { asked++; return gone })

	if err := st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session}); err != nil {
		t.Fatalf("an append before the parent went: %v", err)
	}
	// An old run an eviction would remove, were it not standing down.
	const old = "sess-old"
	if err := st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_9", SessionID: old}); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1_700_000_000_000)
	files, err := os.ReadDir(st.RunDir(old))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		stale := now.Add(-2 * evictGrace)
		if err := os.Chtimes(filepath.Join(st.RunDir(old), f.Name()), stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	gone = true
	appends := []struct {
		name string
		fn   func() error
	}{
		{"declaration", func() error {
			return st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_2", SessionID: session})
		}},
		{"execution", func() error {
			return st.AppendExecution(Execution{Type: TypeExecution, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session})
		}},
		{"terminal", func() error {
			return st.AppendTerminal(Terminal{Type: TypeTerminal, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session})
		}},
		{"spill terminal", func() error {
			return st.SpillTerminal(Terminal{Type: TypeTerminal, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session})
		}},
		{"coverage", func() error {
			return st.AppendCoverage(Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session})
		}},
		{"probe marker", func() error { return st.MarkProbe("sess-other", now) }},
		{"eviction", func() error { _, err := st.Evict(1, session, now); return err }},
	}
	for _, a := range appends {
		before := asked
		if err := a.fn(); !errors.Is(err, ErrStoodDown) {
			t.Errorf("%s after stand-down: %v, want ErrStoodDown", a.name, err)
		}
		if asked == before {
			t.Errorf("%s did not ask whether to stand down", a.name)
		}
	}

	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 1 || len(run.Executions) != 0 || len(run.Terminals) != 0 || len(run.Coverage) != 0 {
		t.Errorf("after stand-down the run holds %d declarations, %d executions, %d terminals, %d coverage; want 1, 0, 0, 0",
			len(run.Declarations), len(run.Executions), len(run.Terminals), len(run.Coverage))
	}
	if _, err := os.Stat(st.RunDir("sess-other")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the refused probe marker still created its run directory: %v", err)
	}
	if _, err := os.Stat(st.RunDir(old)); err != nil {
		t.Errorf("the refused eviction removed a run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.Root(), FileGaps)); err == nil {
		if gaps, err := st.ReadGaps(); err != nil || len(gaps) != 0 {
			t.Errorf("the refused eviction left gaps %v (%v)", gaps, err)
		}
	}
}

// TestREC55b_TheSupervisorsRecordNeverJoinsATornLine: a child killed in the
// middle of a coverage append leaves a last line with no newline. The
// supervisor's record goes after a newline it writes first, under the same
// lock, so the torn fragment reads as one skipped line and the record reads
// whole; on a file that ends cleanly, or does not exist yet, no extra
// newline is written (REC-55; tech 1.2.11's torn-tail rule).
func TestREC55b_TheSupervisorsRecordNeverJoinsATornLine(t *testing.T) {
	const session = "sess-torn"
	killed := ReasonHookKilled
	rec := Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session, Phase: PhaseCall, State: StateUnverified, Reason: &killed}

	for _, c := range []struct {
		name, before string
		wantLines    int
	}{
		{"no file", "", 1},
		{"clean tail", `{"type":"coverage","schema_version":3,"session_id":"sess-torn","phase":"call","state":"verified"}` + "\n", 2},
		{"torn tail", `{"type":"coverage","schema_version":3,"sess`, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			st, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(st.RunDir(session), FileCoverage)
			if c.before != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(c.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.AppendSupervisorCoverage(rec); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(body, []byte(c.before)) {
				t.Fatalf("the append rewrote what was there:\n%s", body)
			}
			lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
			if len(lines) != c.wantLines {
				t.Fatalf("%d lines, want %d:\n%s", len(lines), c.wantLines, body)
			}
			var got Coverage
			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
				t.Fatalf("the supervisor's record does not read whole: %v\n%s", err, body)
			}
			if got.Reason == nil || *got.Reason != ReasonHookKilled {
				t.Errorf("read back reason %v, want %s", got.Reason, ReasonHookKilled)
			}
			run, err := st.ReadRun(session)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, cov := range run.Coverage {
				if cov.Reason != nil && *cov.Reason == ReasonHookKilled {
					found = true
				}
			}
			if !found {
				t.Errorf("the run reader does not see the supervisor's record: %+v", run.Coverage)
			}
		})
	}
}

// TestRES23b_TheSupervisorLogRefusesAnOverCapWrite: supervisor.ndjson is a
// side file under a 256 KiB cap (RES-23). A line that fits is appended as one
// line of codes; a line that would take the file over the cap is refused with
// ErrSideFileCap and the file is left exactly as it was.
func TestRES23b_TheSupervisorLogRefusesAnOverCapWrite(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	line := SupervisorLine{AtUnixMS: 1, Entry: "post", Event: "PostToolUse", Outcome: OutcomeExitNonZero, ChildExit: 2}
	if err := st.AppendSupervisorLine(line); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root(), FileSupervisor)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at_unix_ms":1,"entry":"post","event":"PostToolUse","outcome":"exit_nonzero","child_exit":2,"signal":""}` + "\n"
	if string(body) != want {
		t.Errorf("supervisor line\n got %s\nwant %s", body, want)
	}

	// Fill the file to one byte short of the cap: the next line cannot fit.
	pad := bytes.Repeat([]byte("x"), SupervisorLogCap-len(body)-1)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(pad); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendSupervisorLine(line); !errors.Is(err, ErrSideFileCap) {
		t.Errorf("an over-cap supervisor line: %v, want ErrSideFileCap", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("a refused line changed the file: %d bytes before, %d after", len(before), len(after))
	}
	if len(after) > SupervisorLogCap {
		t.Errorf("the supervisor log is %d bytes, over its %d-byte cap", len(after), SupervisorLogCap)
	}
}
