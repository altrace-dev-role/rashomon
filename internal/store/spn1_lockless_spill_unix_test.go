//go:build unix

package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The spill file's grammar is its writer's (#104's ruling): records separated
// by one or more newlines. A spill whose own lock is held past spillBudget
// appends without it, and that write leads with a newline so it cannot glue
// onto a torn tail it has no lock to check (appendLine's appendWithoutLock
// branch). Every line under contention is then preceded by a blank one, and
// every reader of the file skips blank lines there (forEachLine). These tests
// take that branch deterministically: the spill file's lock is held by a
// handle of the test's own for every write, so each one gives up at its
// budget, whatever the load. Unix-only: Windows' mandatory lock refuses the
// lockless write itself.

// holdSpill takes the session's spill lock through a handle of the test's own,
// and returns its release, which must run before the file is read.
func holdSpill(t *testing.T, st *Store, session string) (path string, release func()) {
	t.Helper()
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, FileSpill)
	unlock, _, err := lockFileWait(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatalf("taking the spill lock: %v", err)
	}
	return path, unlock
}

// spillThree spills a terminal, an execution and a terminal, each through the
// lockless branch while the test holds the lock.
func spillThree(t *testing.T, st *Store, session string) {
	t.Helper()
	spillAt(t, st, session, "toolu_a", 1000)
	if err := st.SpillExecution(Execution{Type: TypeExecution, SchemaVersion: SchemaVersion, RecordedAtMS: 2000,
		ToolUseID: "toolu_b", SessionID: session, ToolName: "Bash"}); err != nil {
		t.Fatal(err)
	}
	spillAt(t, st, session, "toolu_c", 3000)
}

// TestSPN1_ALocklessSpillReadsBack: three spills made without the lock leave
// a blank line before each, and every reader of the spill file gets all three
// back and counts nothing unreadable: ReadRun (report and verify),
// ReadRunConsistent (the digest), readLinesAt (forget and eviction, which
// rewrite the file from what it returns) and Fold (the Stop's streaming read).
// Break: forEachLine hands a blank line over as a line, which it reports as
// one over the limit.
func TestSPN1_ALocklessSpillReadsBack(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-lockless"
	path, release := holdSpill(t, st, session)
	spillThree(t, st, session)
	release()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("\n")) || bytes.Count(b, []byte("\n\n")) != 2 {
		t.Fatalf("premise broken: the held lock did not send each spill down the lockless branch: %q", b)
	}

	reads := []struct {
		name string
		read func() (*Run, error)
	}{
		{"ReadRun", func() (*Run, error) { return st.ReadRun(session) }},
		{"ReadRunConsistent", func() (*Run, error) { return st.ReadRunConsistent(session, 10*time.Millisecond) }},
	}
	for _, r := range reads {
		run, err := r.read()
		if err != nil {
			t.Fatalf("%s: %v", r.name, err)
		}
		if len(run.Terminals) != 2 || len(run.Executions) != 1 || run.Skipped != 0 {
			t.Errorf("%s read %d terminals, %d executions and %d unreadable lines, want 2, 1 and 0",
				r.name, len(run.Terminals), len(run.Executions), run.Skipped)
		}
	}

	lines, err := readLinesAt(path)
	if err != nil || len(lines) != 3 {
		t.Errorf("readLinesAt: %d lines, err %v; want the 3 records and no error", len(lines), err)
	}

	folded, unreadable := 0, 0
	off, size, err := Fold(path, 0, func(line []byte) {
		if line == nil {
			unreadable++
			return
		}
		folded++
	})
	if err != nil || folded != 3 || unreadable != 0 || off != size || size != int64(len(b)) {
		t.Errorf("Fold: %d lines, %d unreadable, offset %d of size %d (file %d bytes), err %v; want 3, 0 and the whole file",
			folded, unreadable, off, size, len(b), err)
	}
}

// TestSPN1_ALocklessSpillDoesNotGlueOntoATornTail: the leading newline is the
// lockless write's guard. A spill file that ends in a torn line -- an append
// that died mid-write -- still gets each spilled record back as its own line;
// only the torn one is unreadable. Break: the lockless write drops its
// leading newline, and the first spill joins the torn line and is lost with it.
func TestSPN1_ALocklessSpillDoesNotGlueOntoATornTail(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-torn"
	path, release := holdSpill(t, st, session)
	if err := os.WriteFile(path, []byte(`{"type":"terminal","schema_ver`), fileMode); err != nil {
		t.Fatal(err)
	}
	spillThree(t, st, session)
	release()

	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Terminals) != 2 || len(run.Executions) != 1 || run.Skipped != 1 {
		t.Errorf("read %d terminals, %d executions and %d unreadable lines after a torn tail, want 2, 1 and 1 (the torn line alone)",
			len(run.Terminals), len(run.Executions), run.Skipped)
	}
	for _, term := range run.Terminals {
		if term.Seq != nil || (term.ToolUseID != "toolu_a" && term.ToolUseID != "toolu_c") {
			t.Errorf("spilled terminal %+v, want toolu_a or toolu_c with a null seq", term)
		}
	}
}
