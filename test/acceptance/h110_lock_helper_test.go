//go:build unix || windows

package acceptance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// H-110 -- the store's lock is one exported helper, and it keeps its contract
// for a caller outside the package.
//
// internal/store took its lock through an unexported lockFile, so the
// experimental memory build, which needs the same bounded exclusive lock on a
// file of its own, carried a copy of all three platform files. Two copies of a
// lock drift apart: a fix found in one -- the O_RDWR a Windows handle needs
// before LockFileEx will lock it was found that way -- does not reach the
// other. store.LockFile is now the one helper, and the store takes every lock
// it takes through it.
//
// What a second package depends on, held down here from outside the package:
// a second descriptor on the same file cannot take the lock while the first
// holds it; the contender gives up with store.ErrLockTimeout no sooner than
// the budget it passed and long before the store's own 2 s default; once the
// holder releases, the contender takes it; and a lock that fails for a reason
// other than contention returns that error at once. The last matters because
// the hooks record a timeout as lock_timeout: a broken descriptor reported as
// a busy lock would be a claim about contention nobody measured.
//
// Breaks (test/mutation/mutants/H-110_lock-export.py): the release leaves the
// lock held; every lock error reads as contention; and, judged here as well as
// by H-16's own test, the lock is shared rather than exclusive, ignores the
// caller's budget, or gives up before it.
//
// The ceilings are generous on purpose. The only bound that is tight is the
// floor -- a contended call returning before its budget -- and that one is
// decided by the deadline check, not by the scheduler.

func TestH110_TheLockExcludesASecondDescriptorForTheCallersBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	held := openLockable(t, path)
	contender := openLockable(t, path)

	unlock, err := store.LockFile(held, time.Second)
	if err != nil {
		t.Fatalf("LockFile on a file nobody holds: %v", err)
	}

	const budget = 50 * time.Millisecond
	start := time.Now()
	release, err := store.LockFile(contender, budget)
	waited := time.Since(start)
	if err == nil {
		release()
		t.Fatal("a second descriptor took the lock while the first held it")
	}
	if !errors.Is(err, store.ErrLockTimeout) {
		t.Fatalf("a contended LockFile returned %v, want store.ErrLockTimeout", err)
	}
	if waited < budget {
		t.Fatalf("a contended LockFile gave up after %v, before the %v budget it was given", waited, budget)
	}
	// Half the store's 2 s default and twenty times the budget: a LockFile
	// that waited its package default instead of what it was passed lands
	// past this, and scheduler noise does not.
	if waited > time.Second {
		t.Fatalf("a contended LockFile waited %v for a %v budget", waited, budget)
	}

	unlock()

	release, err = store.LockFile(contender, budget)
	if err != nil {
		t.Fatalf("LockFile after the holder released: %v", err)
	}
	release()
}

func TestH110_AFailureThatIsNotContentionIsNotATimeout(t *testing.T) {
	f := openLockable(t, filepath.Join(t.TempDir(), "lock"))

	// The positive twin: the same descriptor locks while it is open, so the
	// refusal below is about the closed descriptor and not about the file or
	// the platform.
	unlock, err := store.LockFile(f, time.Second)
	if err != nil {
		t.Fatalf("LockFile on an open descriptor: %v", err)
	}
	unlock()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	release, err := store.LockFile(f, 50*time.Millisecond)
	if err == nil {
		release()
		t.Fatal("LockFile took a lock through a closed descriptor")
	}
	if errors.Is(err, store.ErrLockTimeout) {
		t.Fatalf("LockFile on a closed descriptor returned %v: a broken lock reported as a busy one", err)
	}
}

// openLockable opens path the way the store opens a file it locks: with read
// and write access, which LockFileEx requires of a handle (store.LockFile).
func openLockable(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// TestH110_ABudgetOfZeroIsATryLock: a budget of 0 tries the lock once and
// returns at once, store.ErrLockTimeout under contention and the lock when it
// is free. The history fold takes it that way: a render that finds the fold
// running skips it rather than queueing behind it. Reading 0 as "no budget,
// wait until free" -- as some lock APIs do -- would hang that render, so the
// contended call has 10 s to return before this fails rather than hangs.
func TestH110_ABudgetOfZeroIsATryLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	held := openLockable(t, path)
	contender := openLockable(t, path)

	unlock, err := store.LockFile(held, time.Second)
	if err != nil {
		t.Fatalf("LockFile on a file nobody holds: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		release, err := store.LockFile(contender, 0)
		if err == nil {
			release()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, store.ErrLockTimeout) {
			t.Fatalf("a contended LockFile with budget 0 returned %v, want store.ErrLockTimeout", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a contended LockFile with budget 0 waited for the lock instead of trying it once")
	}
	unlock()
	release, err := store.LockFile(contender, 0)
	if err != nil {
		t.Fatalf("LockFile with budget 0 on a free lock: %v", err)
	}
	release()
}
