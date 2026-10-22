package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRubricPERF2_OneDeadlineBoundsEveryLockWait: a hook appends up to three
// times, and each append waited out the full two-second lock budget on its
// own, so one invocation could wait six seconds against a five-second hook
// timeout. With LimitLockWaits the waits share one deadline: here the first
// append spends it, and the second tries its lock once and gives up.
func TestRubricPERF2_OneDeadlineBoundsEveryLockWait(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-deadline"
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{FileRecords, FileCoverage} {
		unlock, _, err := lockFileWait(openForAppend(t, filepath.Join(dir, name)), lockBudget)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
	}

	const total = 300 * time.Millisecond
	st.LimitLockWaits(total)
	start := time.Now()
	if err := st.AppendDeclaration(testDecl(session, "toolu_1")); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("AppendDeclaration on a held lock: %v, want ErrLockTimeout", err)
	}
	first := time.Since(start)
	if err := st.AppendCoverage(Coverage{SessionID: session}); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("AppendCoverage on a held lock: %v, want ErrLockTimeout", err)
	}
	all := time.Since(start)
	if first < total {
		t.Errorf("the first wait gave up after %v, before the %v the deadline left it", first, total)
	}
	if all > time.Second {
		t.Errorf("two appends on held locks waited %v under a %v deadline; each waited its own %v budget", all, total, lockBudget)
	}
}

// TestRubricPERF2_LockWaitIsTheBudgetUntilLimited: a Store nobody limited --
// probe's, recap's, a command's -- waits each lock's own budget, and a
// limited one never waits longer than that budget or than the time left.
func TestRubricPERF2_LockWaitIsTheBudgetUntilLimited(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := st.lockWait(lockBudget); got != lockBudget {
		t.Errorf("lockWait on an unlimited Store is %v, want %v", got, lockBudget)
	}
	st.LimitLockWaits(time.Hour)
	if got := st.lockWait(spillBudget); got != spillBudget {
		t.Errorf("lockWait(%v) with an hour left is %v", spillBudget, got)
	}
	st.LimitLockWaits(-time.Second)
	if got := st.lockWait(lockBudget); got != 0 {
		t.Errorf("lockWait past the deadline is %v, want 0: one try and no wait", got)
	}
}
