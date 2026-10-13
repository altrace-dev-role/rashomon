//go:build unix

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The unix half of test-11. A held flock never refuses a read, so the
// Windows refusal (ERROR_LOCK_VIOLATION) cannot happen here; a directory
// stands in for it. Opening one succeeds, flock takes on it, and reading it
// fails with EISDIR, so a predicate that calls EISDIR a refusal drives the
// real open, lock, scan and refusal handling end to end. What the Windows
// tests (lock_refused_windows_test.go) add is only the OS fact that a
// mandatory lock refuses the read with ERROR_LOCK_VIOLATION.

func refusesAsEISDIR(err error) bool { return errors.Is(err, syscall.EISDIR) }

// refusingPath returns a path whose read is refused under refusesAsEISDIR.
func refusingPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "refuses")
	if err := os.Mkdir(path, dirMode); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRubricTEST11_ARefusedReadUnderAHeldLockCountsAsSkipped: when the
// writer's lock is held and the reader's unlocked fallback read is refused,
// the refusal reaches fn as one nil line, which every caller counts as a
// line it could not read, and the read does not fail.
func TestRubricTEST11_ARefusedReadUnderAHeldLockCountsAsSkipped(t *testing.T) {
	path := refusingPath(t)
	holder, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close() //nolint:errcheck // read-only
	unlock, err := lockFile(holder, lockBudget)
	if err != nil {
		t.Fatalf("taking the lock: %v", err)
	}
	defer unlock()

	var lines [][]byte
	err = eachLineTryLock(path, time.Millisecond, refusesAsEISDIR, func(line []byte) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("eachLineTryLock under a held lock with a refused read: %v, want the refusal counted", err)
	}
	if len(lines) != 1 || lines[0] != nil {
		t.Errorf("fn got %d lines (%q), want exactly one nil line for the refused read", len(lines), lines)
	}
}

// TestRubricTEST11_ARefusalWithTheLockTakenIsAnError: a reader that got the
// lock cannot be refused by it, so a read error then is something else and
// is returned, not counted as a skipped line.
func TestRubricTEST11_ARefusalWithTheLockTakenIsAnError(t *testing.T) {
	path := refusingPath(t)
	var lines [][]byte
	err := eachLineTryLock(path, lockBudget, refusesAsEISDIR, func(line []byte) { lines = append(lines, line) })
	if !errors.Is(err, syscall.EISDIR) {
		t.Fatalf("eachLineTryLock with its lock taken: err %v, want the read error itself", err)
	}
	if len(lines) != 0 {
		t.Errorf("fn got %d lines, want none: an error the lock did not cause is not a skipped line", len(lines))
	}
}

// TestRubricTEST11_ARefusedGapsReadCountsAsSkipped: gaps.ndjson is read
// unlocked, and on Windows a forget or an eviction holding the gaps lock
// refuses that read. ReadGaps counts the refusal as one gap line it could
// not read -- every session then reads unverified (REC-33) -- rather than
// failing digest.Build. ForgottenHost's read treats no error as a refusal:
// an unread gaps file there must fail the report, not read as "nothing was
// forgotten" and show a host the user removed.
func TestRubricTEST11_ARefusedGapsReadCountsAsSkipped(t *testing.T) {
	path := refusingPath(t)

	gaps, skipped, err := readGapsAt(path, refusesAsEISDIR)
	if err != nil {
		t.Fatalf("readGapsAt with a refused read: %v, want the refusal counted", err)
	}
	if skipped != 1 || len(gaps) != 0 {
		t.Errorf("got %d gaps and %d skipped, want none read and one skipped", len(gaps), skipped)
	}

	if _, _, err := readGapsAt(path, neverRefused); !errors.Is(err, syscall.EISDIR) {
		t.Errorf("readGapsAt with no refusal predicate: err %v, want the read error itself", err)
	}
}

// TestRubricTEST11_TheConsistentReadCountsARefusalAtItsCallSite drives the
// refusal through ReadRunConsistent itself rather than eachLineTryLock: a
// held records lock, a read the store's own refusal predicate calls refused,
// and the run reads with that file counted as one skipped line. A
// ReadRunConsistent that passed a predicate of its own instead of the
// store's would fail the read here, as it would on Windows.
func TestRubricTEST11_TheConsistentReadCountsARefusalAtItsCallSite(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st.readRefusal = refusesAsEISDIR
	const session = "sess-refused"
	recordsPath := filepath.Join(st.RunDir(session), FileRecords)
	if err := os.MkdirAll(recordsPath, dirMode); err != nil {
		t.Fatal(err)
	}
	holder, err := os.Open(recordsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close() //nolint:errcheck // read-only
	unlock, err := lockFile(holder, lockBudget)
	if err != nil {
		t.Fatalf("taking the records lock: %v", err)
	}
	defer unlock()

	run, err := st.ReadRunConsistent(session, time.Millisecond)
	if err != nil {
		t.Fatalf("ReadRunConsistent with its records read refused: %v, want the refusal counted", err)
	}
	if run.Skipped != 1 {
		t.Errorf("run has %d skipped lines, want the refused records file counted as one", run.Skipped)
	}
}

// TestRubricTEST11_TheGapsReadCountsARefusalAtItsCallSite is the same for
// ReadGaps: a gaps read the store's refusal predicate calls refused is one
// gap line that could not be read, not a failed read.
func TestRubricTEST11_TheGapsReadCountsARefusalAtItsCallSite(t *testing.T) {
	root := t.TempDir()
	st, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st.readRefusal = refusesAsEISDIR
	if err := os.Mkdir(filepath.Join(root, FileGaps), dirMode); err != nil {
		t.Fatal(err)
	}

	gaps, skipped, err := st.ReadGaps()
	if err != nil {
		t.Fatalf("ReadGaps with its read refused: %v, want the refusal counted", err)
	}
	if skipped != 1 || len(gaps) != 0 {
		t.Errorf("got %d gaps and %d skipped, want none read and one skipped", len(gaps), skipped)
	}
}

// refuseRecordsRead makes path's read refused, as a held records lock does
// on Windows: path becomes a directory and st counts EISDIR as a refusal.
func refuseRecordsRead(t *testing.T, st *Store, path string) func() {
	t.Helper()
	if err := os.Mkdir(path, dirMode); err != nil {
		t.Fatal(err)
	}
	st.readRefusal = refusesAsEISDIR
	return func() {}
}
