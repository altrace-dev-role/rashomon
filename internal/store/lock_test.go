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

func openForAppend(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, appendFlags, fileMode)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
