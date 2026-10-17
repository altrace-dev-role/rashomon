//go:build unix || windows

package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// holdFor takes path's lock through a handle of its own and releases it
// after d, as a writer in another process holding it that long would, or when
// the test ends, whichever is first. It returns the held file, so a test can
// name its descriptor.
//
// The release at the end of the test is registered after openForAppend's
// Close, so it runs first, while the descriptor is still the held file's.
// Once, because the timer's goroutine and the cleanup can both release, and
// the cleanup must not return while the timer's unlock is still running.
func holdFor(t *testing.T, path string, d time.Duration) *os.File {
	t.Helper()
	f := openForAppend(t, path)
	unlock, _, err := lockFile(f, lockBudget)
	if err != nil {
		t.Fatalf("taking the lock on %s: %v", path, err)
	}
	var once sync.Once
	release := func() { once.Do(unlock) }
	timer := time.AfterFunc(d, release)
	t.Cleanup(func() {
		timer.Stop()
		release()
	})
	return f
}

// TestHoldForEndsWithItsTest: a hold its test gives up on ends with the test.
// lockFile's unlock releases a descriptor by its number, so a hold whose
// timer fired after its test had closed the held file released the lock of
// whichever file had been given that number since.
// TestNoteLockWaitRespectsTheSharedDeadline gives up 5 ms into a 200 ms hold;
// under -race the tests after it take about the rest, and the timer fired
// into TestRubricPERF1_AGiveUpLandsAtItsBudget, whose first descriptor had
// the number, so its second descriptor took the lock the first one held.
func TestHoldForEndsWithItsTest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a new handle is not reliably given a closed one's value, so the reuse this test needs cannot be arranged")
	}
	const hold = 20 * time.Millisecond
	held := -1
	t.Run("gives up inside the hold", func(t *testing.T) {
		held = int(holdFor(t, filepath.Join(t.TempDir(), FileRecords), hold).Fd())
	})

	// open(2) takes the lowest free descriptor, so opening until one comes
	// back with the held file's number finds it however many lower ones are
	// free.
	path := filepath.Join(t.TempDir(), FileRecords)
	f := openForAppend(t, path)
	for i := 0; i < 64 && int(f.Fd()) != held; i++ {
		f = openForAppend(t, path)
	}
	if int(f.Fd()) != held {
		t.Fatalf("no new descriptor was given the held file's number %d", held)
	}
	unlock, _, err := lockFile(f, lockBudget)
	if err != nil {
		t.Fatalf("lockFile on an uncontended file: %v", err)
	}
	defer unlock()

	time.Sleep(5 * hold)
	release, _, err := lockFile(openForAppend(t, path), time.Millisecond)
	if err == nil {
		release()
		t.Fatal("a hold that outlived its test released the lock of the file that reused its descriptor")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("contended lockFile returned %v, want ErrLockTimeout", err)
	}
}

// TestRubricPERF7_LockFileReportsItsWait: lockFile says how long it waited.
// A lock taken at once waited nothing; one taken behind a 30 ms hold waited
// that hold out; one that gave up waited its budget.
func TestRubricPERF7_LockFileReportsItsWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileRecords)

	unlock, waited, err := lockFile(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatalf("lockFile on an uncontended file: %v", err)
	}
	unlock()
	if waited != 0 {
		t.Errorf("an uncontended lock reports a %v wait, want none", waited)
	}

	const hold = 30 * time.Millisecond
	holdFor(t, path, hold)
	unlock, waited, err = lockFile(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatalf("lockFile behind a %v hold: %v", hold, err)
	}
	unlock()
	if waited < hold-5*time.Millisecond || waited > lockBudget {
		t.Errorf("a lock taken behind a %v hold reports a %v wait", hold, waited)
	}

	holder, _, err := lockFile(openForAppend(t, path), lockBudget)
	if err != nil {
		t.Fatal(err)
	}
	defer holder()
	const budget = 20 * time.Millisecond
	_, waited, err = lockFile(openForAppend(t, path), budget)
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("lockFile under a held lock: %v, want ErrLockTimeout", err)
	}
	if waited < budget {
		t.Errorf("a lock that gave up reports a %v wait, under its %v budget", waited, budget)
	}
}

// TestRubricPERF7_AContendedInvocationNotesItsLongestWait: lock contention
// used to be invisible until it cost a record. An invocation whose appends
// waited notes how long in its run, the run keeps the longest any one
// invocation waited, and LongestLockWait, which status shows, finds it. An
// invocation that never waited notes nothing.
func TestRubricPERF7_AContendedInvocationNotesItsLongestWait(t *testing.T) {
	root := t.TempDir()
	const session, other = "sess-contended", "sess-other"
	invocation := func(session string, hold time.Duration) {
		t.Helper()
		st, err := Open(root)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if hold > 0 {
			holdFor(t, filepath.Join(st.RunDir(session), FileRecords), hold)
		}
		if err := st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion,
			ToolUseID: "toolu_x", SessionID: session}); err != nil {
			t.Fatalf("AppendDeclaration: %v", err)
		}
		if err := st.AppendCoverage(Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion,
			SessionID: session, Phase: PhaseCall, State: StateVerified}); err != nil {
			t.Fatalf("AppendCoverage: %v", err)
		}
	}
	longest := func() (time.Duration, string) {
		t.Helper()
		st, err := Open(root)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		d, in, err := st.LongestLockWait()
		if err != nil {
			t.Fatalf("LongestLockWait: %v", err)
		}
		return d, in
	}

	invocation(session, 0)
	if d, in := longest(); d != 0 || in != "" {
		t.Fatalf("after an uncontended invocation the longest wait is %v in %q, want none", d, in)
	}

	invocation(session, 60*time.Millisecond)
	first, in := longest()
	if first < 55*time.Millisecond || in != segment(session) {
		t.Fatalf("after a 60ms hold the longest wait is %v in %q, want at least the hold, in %q", first, in, segment(session))
	}

	invocation(session, 10*time.Millisecond)
	if d, _ := longest(); d != first {
		t.Errorf("a shorter wait replaced the run's longest: %v, want %v kept", d, first)
	}

	// A later run's shorter wait does not hide an earlier run's longer one.
	invocation(other, 0)
	invocation(other, 10*time.Millisecond)
	if d, in := longest(); d != first || in != segment(session) {
		t.Errorf("with a shorter wait in a later run the longest is %v in %q, want %v in %q", d, in, first, segment(session))
	}
}
