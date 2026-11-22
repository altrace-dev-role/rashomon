package store

import (
	"os"
	"path/filepath"
	"testing"
)

// AfterFrontierRemoved, ComposeHeld and ComposeBetween are the compose
// seams, for the external tests that drive a forget, an append or a held
// lock into their windows with the session graph's Compose, which this
// package cannot import.
var (
	AfterFrontierRemoved = &afterFrontierRemoved
	ComposeHeld          = &composeHeld
	ComposeBetween       = &composeBetween
)

// HoldRecordsLock takes session's records lock on a handle of its own, as
// another process's append holds it, and returns its release.
func HoldRecordsLock(t *testing.T, st *Store, session string) func() {
	t.Helper()
	f, err := os.Open(filepath.Join(st.RunDir(session), FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	unlock, _, err := lockCounted(f, lockBudget)
	if err != nil {
		f.Close() //nolint:errcheck // read-only
		t.Fatal(err)
	}
	return func() {
		unlock()
		f.Close() //nolint:errcheck // read-only
	}
}
