//go:build unix

package hook

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// holdLocks takes the store's advisory lock on each named file, as writers
// in other processes would hold them, until the test ends.
func holdLocks(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
	}
}

// perf2Waits stands in for LockWaitTotal, so a test that waits out the
// deadline costs a fraction of a second.
const perf2Waits = 300 * time.Millisecond

// perf2Bound is what an invocation may take under perf2Waits: the deadline,
// one spill budget, and room for the work around them. Without the shared
// deadline each held lock costs its own two seconds.
const perf2Bound = time.Second

// TestRubricPERF2_AHookWaitsOneDeadlineForAllItsLocks: a declaration that
// waits out the lock, then a terminal and a coverage record, wait for their
// locks against one deadline, not three budgets in a row.
func TestRubricPERF2_AHookWaitsOneDeadlineForAllItsLocks(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	dir := st.RunDir("sess-caps")
	holdLocks(t, filepath.Join(dir, store.FileRecords), filepath.Join(dir, store.FileCoverage))

	h := New(st, time.Now)
	h.lockWaits = perf2Waits
	start := time.Now()
	err := h.Capture(strings.NewReader(capPayload(t, "tool_use_id", "toolu_deadline")))
	h.Close(&Signals{}, err)
	took := time.Since(start)

	if !errors.Is(err, store.ErrLockTimeout) {
		t.Fatalf("Capture on a held records lock: %v, want ErrLockTimeout", err)
	}
	if took < perf2Waits || took > perf2Bound {
		t.Errorf("the invocation took %v with every lock held; want between %v and %v", took, perf2Waits, perf2Bound)
	}
	run, err := st.ReadRun("sess-caps")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Terminals) != 1 || run.Terminals[0].ToolUseID != "toolu_deadline" {
		t.Errorf("terminals %+v, want the spilled terminal naming the dropped call", run.Terminals)
	}
}

// TestRubricPERF2_APostWaitsOneDeadlineForAllItsLocks is the same for a
// PostToolUse invocation: its execution spills, and its coverage record gets
// what is left of the deadline.
func TestRubricPERF2_APostWaitsOneDeadlineForAllItsLocks(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	dir := st.RunDir("sess-caps")
	holdLocks(t, filepath.Join(dir, store.FileRecords), filepath.Join(dir, store.FileCoverage))

	p := NewPost(st, time.Now)
	p.lockWaits = perf2Waits
	start := time.Now()
	err := p.Capture(strings.NewReader(capPayload(t, "hook_event_name", "PostToolUse")))
	p.Close(&Signals{}, err)
	took := time.Since(start)

	if err != nil {
		t.Fatalf("Capture: %v; an execution that loses the lock spills", err)
	}
	if took < perf2Waits || took > perf2Bound {
		t.Errorf("the invocation took %v with every lock held; want between %v and %v", took, perf2Waits, perf2Bound)
	}
	run, err := st.ReadRun("sess-caps")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Executions) != 1 || run.Executions[0].Seq != nil {
		t.Errorf("executions %+v, want the one spilled execution", run.Executions)
	}
}

// TestRubricPERF2_AProbeWaitsOneDeadlineForAllItsLocks: a session boundary
// writes a coverage record and then evicts under the gaps lock, against one
// deadline too.
func TestRubricPERF2_AProbeWaitsOneDeadlineForAllItsLocks(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	holdLocks(t, filepath.Join(st.RunDir("sess-probe"), store.FileCoverage), filepath.Join(st.Root(), store.FileGaps))

	start := time.Now()
	err := runProbe(&Signals{}, store.PhaseEnd, strings.NewReader(`{"session_id":"sess-probe"}`), st, time.Now, perf2Waits)
	took := time.Since(start)
	if !errors.Is(err, store.ErrLockTimeout) {
		t.Errorf("eviction on a held gaps lock: %v, want ErrLockTimeout", err)
	}
	if took < perf2Waits || took > perf2Bound {
		t.Errorf("the probe took %v with every lock held; want between %v and %v", took, perf2Waits, perf2Bound)
	}
}

// TestRubricPERF2_LockWaitTotalLeavesTheTimeoutRoom: the shared deadline is
// what keeps an invocation's lock waits inside the hook timeout, so it has
// to sit under it with room for the rest of the invocation.
func TestRubricPERF2_LockWaitTotalLeavesTheTimeoutRoom(t *testing.T) {
	timeout := time.Duration(install.Timeout) * time.Second
	// At least the two seconds one append waits on its own, so a declaration
	// keeps its whole budget, and a second under the timeout for the rest.
	if LockWaitTotal+time.Second > timeout || LockWaitTotal < 2*time.Second {
		t.Errorf("LockWaitTotal %v against a %v hook timeout", LockWaitTotal, timeout)
	}
}
