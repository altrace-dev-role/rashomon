package store

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestUnlockAfterCloseReleasesNothing: an unlock that runs after its file was
// closed must not release the lock of the file that has been given the same
// descriptor number since. Every caller unlocks before it closes today; this
// holds the release to the file itself, so a later caller that gets the order
// wrong loses its own lock and nobody else's (CWE-672).
func TestUnlockAfterCloseReleasesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a new handle is not reliably given a closed one's value, so the reuse this test needs cannot be arranged")
	}
	dir := t.TempDir()
	first := openForAppend(t, filepath.Join(dir, "first"))
	unlockFirst, _, err := lockFileWait(first, lockBudget)
	if err != nil {
		t.Fatalf("lockFileWait on an uncontended file: %v", err)
	}
	held := int(first.Fd())
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// open(2) takes the lowest free descriptor, so opening until one comes back
	// with the closed file's number finds it however many lower ones are free.
	path := filepath.Join(dir, FileRecords)
	f := openForAppend(t, path)
	for i := 0; i < 64 && int(f.Fd()) != held; i++ {
		f = openForAppend(t, path)
	}
	if int(f.Fd()) != held {
		t.Fatalf("no new descriptor was given the closed file's number %d", held)
	}
	unlock, _, err := lockFileWait(f, lockBudget)
	if err != nil {
		t.Fatalf("lockFileWait on an uncontended file: %v", err)
	}
	defer unlock()

	unlockFirst()
	release, _, err := lockFileWait(openForAppend(t, path), time.Millisecond)
	if err == nil {
		release()
		t.Fatal("an unlock after Close released the lock of the file that reused its descriptor")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("a second descriptor's lock: got %v, want ErrLockTimeout", err)
	}
}
