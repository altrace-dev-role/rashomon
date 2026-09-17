//go:build unix

package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLockFileIsExclusiveAndBounded is the contract every platform's lockFile
// has to honour, written down where a port can be checked against it.
//
// H-16 exercises the lock through 64 processes; this exercises it through two
// descriptors, which is the smallest arrangement that can tell exclusion from
// a function that returns success without locking anything.
func TestLockFileIsExclusiveAndBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileRecords)
	held := openForAppend(t, path)
	contender := openForAppend(t, path)

	unlock, err := lockFile(held, lockBudget)
	if err != nil {
		t.Fatalf("lockFile on an uncontended file: %v", err)
	}

	const budget = 50 * time.Millisecond
	start := time.Now()
	release, err := lockFile(contender, budget)
	waited := time.Since(start)
	if err == nil {
		release()
		t.Fatal("a second descriptor took the lock while the first held it")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("contended lockFile returned %v, want ErrLockTimeout", err)
	}
	if waited < budget {
		t.Fatalf("contended lockFile gave up after %v, before its %v budget", waited, budget)
	}
	// Well under lockBudget: a lockFile that ignored its argument and waited
	// the package default would land here instead.
	if waited > time.Second {
		t.Fatalf("contended lockFile waited %v for a %v budget", waited, budget)
	}

	unlock()

	release, err = lockFile(contender, budget)
	if err != nil {
		t.Fatalf("lockFile after the holder released: %v", err)
	}
	release()
}

// TestAppendExecutionSpillsWhenTheLockIsHeld: an execution record is the only
// record its tool_use_id gets, so a lock it cannot take has to cost it its
// place in the total order and nothing else. A dropped execution reads
// afterwards as a call that never ran, which is a claim about the user's
// session rather than about this store.
func TestAppendExecutionSpillsWhenTheLockIsHeld(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const session = "sess-held"
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockFile(openForAppend(t, filepath.Join(dir, FileRecords)), lockBudget)
	if err != nil {
		t.Fatalf("taking the records lock: %v", err)
	}
	defer unlock()

	rec := Execution{
		Type:          TypeExecution,
		SchemaVersion: SchemaVersion,
		RecordedAtMS:  time.Now().UnixMilli(),
		ToolUseID:     "toolu_held",
		SessionID:     session,
		ToolName:      "Bash",
	}
	if err := st.AppendExecution(rec); err != nil {
		t.Fatalf("AppendExecution while the lock is held: %v", err)
	}

	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatalf("ReadRun: %v", err)
	}
	if len(run.Executions) != 1 {
		t.Fatalf("got %d execution records, want 1: the id lands or nothing says the call ran", len(run.Executions))
	}
	if got := run.Executions[0]; got.ToolUseID != rec.ToolUseID || got.Seq != nil {
		t.Errorf("the spilled execution is %+v, want tool_use_id %q and a null seq", got, rec.ToolUseID)
	}
}

func openForAppend(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, appendFlags, fileMode)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
