//go:build unix

package acceptance

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// holdLock takes the advisory lock the store's writers take on path, as a
// stuck writer in another process would hold it, and returns its release.
func holdLock(t *testing.T, path string) func() {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		t.Fatalf("taking the lock on %s: %v", path, err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			f.Close() // closing the descriptor releases the lock
		}
	}
	t.Cleanup(release)
	return release
}

// TestRubricTEST4_AHookThatCannotTakeTheLockSpillsItsTerminal is the
// lock-timeout branch end to end: a hook whose declaration waits out the
// lock budget on a held records file writes nothing to the ordered stream,
// spills a terminal naming the call, and covers the call with lock_timeout.
// It costs the two-second lock budget.
func TestRubricTEST4_AHookThatCannotTakeTheLockSpillsItsTerminal(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	release := holdLock(t, filepath.Join(e.home, "runs", testSession, "records.ndjson"))

	p := defaultPayload()
	p.ToolUseID = "toolu_held"
	start := time.Now()
	e.mustHook(p.build(t))
	took := time.Since(start)
	release()

	if took < 2*time.Second {
		t.Fatalf("the hook returned after %v, before the lock budget: premise broken", took)
	}
	if got := e.read(testSession, "records.ndjson"); len(got) != 0 {
		t.Errorf("%d records reached the ordered stream past a held lock", len(got))
	}
	spilled := e.read(testSession, "spill.ndjson")
	if len(spilled) != 1 || spilled[0].str("tool_use_id") != "toolu_held" || spilled[0].str("reason") != "lock_timeout" ||
		spilled[0].str("outcome") != "error" || spilled[0].fields["seq"] != nil {
		t.Errorf("spill file holds %v, want one lock_timeout terminal for toolu_held with a null seq", spilled)
	}
	cov := e.coverage(testSession, "call")
	if len(cov) != 1 || cov[0].str("reason") != "lock_timeout" {
		t.Errorf("call coverage %v, want one record carrying lock_timeout", cov)
	}
}
