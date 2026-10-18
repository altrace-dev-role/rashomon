package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestForgetAll_ListingIsUnderTheLock: a caller that lists a baseline
// directory and then forgets each file under a lock taken per file, as
// clearBaselines once did, can miss a project baseline a concurrent render
// establishes in the gap between the listing and this walk reaching it --
// Update does not wait for a scan that already finished listing, so a file
// it creates after that point is simply not in the list, no matter how
// quickly the forget runs afterward. Taking the lock once, before the
// listing, makes that gap impossible: anyone else who wants the lock,
// including a render that would have created a new file, waits for
// ForgetAll to finish first. Pinned here by the structural guarantee rather
// than by racing goroutines: ForgetAll cannot even start listing while the
// lock is held elsewhere.
func TestForgetAll_ListingIsUnderTheLock(t *testing.T) {
	root := t.TempDir()
	if r := Update(root, "/proj", "s1", t0, []string{"a.example"}); r.Err != nil {
		t.Fatalf("establishing a baseline to list: %v", r.Err)
	}
	dir := filepath.Join(root, "baseline")
	release, err := store.LockPath(filepath.Join(dir, lockName))
	if err != nil {
		t.Fatalf("taking the baseline lock ahead of ForgetAll: %v", err)
	}
	defer release()

	if _, _, err := ForgetAll(dir, "a.example"); !errors.Is(err, store.ErrLockTimeout) {
		t.Errorf("ForgetAll while the directory lock is held elsewhere: %v, want "+
			"ErrLockTimeout -- if it could list the directory without the lock, "+
			"a baseline established in the gap before it reached the per-file "+
			"work would never be visited", err)
	}
}

// TestForgetAll_SeesAFileCreatedWhileItWasBlockedOnTheLock makes the race the
// previous test proves impossible concrete and deterministic: a project
// baseline that comes into existence WHILE ForgetAll is waiting for the
// lock must still be visited once it gets in. A version that lists the
// directory before taking the lock would have already decided the file list
// by the time the lock opens up, and would walk past this file never having
// known it was there -- not because it raced and lost, but because listing
// before locking means there is nothing to race: the file simply postdates
// the decision of what to look at.
func TestForgetAll_SeesAFileCreatedWhileItWasBlockedOnTheLock(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := store.LockPath(filepath.Join(dir, lockName))
	if err != nil {
		t.Fatalf("taking the baseline lock ahead of ForgetAll: %v", err)
	}

	done := safe.Go(func() {
		cleared, _, err := ForgetAll(dir, "a.example")
		if err != nil {
			t.Errorf("ForgetAll: %v", err)
		}
		if cleared != 1 {
			t.Errorf("cleared = %d, want 1: a baseline created while ForgetAll waited "+
				"for the lock was not visited once it got in", cleared)
		}
	}, func(err error) { t.Errorf("ForgetAll panicked: %v", err) })
	// Long enough that ForgetAll has reached whatever it does ahead of
	// blocking on the lock this goroutine holds -- an unlocked os.ReadDir,
	// in the version this pins against, finishes in microseconds.
	time.Sleep(50 * time.Millisecond)
	writeLegacyBaseline(t, root, "/new", map[string]entry{
		"a.example": {FirstSeenSession: "s1", FirstSeenAtUnixMS: 1},
	})
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ForgetAll did not finish after the lock it was waiting for was released")
	}
}

// TestForgetAll_ClearsEveryProjectAtomically pins the behaviour the refactor
// must not break: multiple projects, one of them not naming the host at all,
// all cleared by one call.
func TestForgetAll_ClearsEveryProjectAtomically(t *testing.T) {
	root := t.TempDir()
	Update(root, "/p1", "s1", t0, []string{"a.example"})
	Update(root, "/p2", "s2", t0, []string{"a.example", "b.example"})
	Update(root, "/p3", "s3", t0, []string{"b.example"})

	cleared, had, err := ForgetAll(filepath.Join(root, "baseline"), "a.example")
	if err != nil {
		t.Fatalf("ForgetAll: %v", err)
	}
	if !had {
		t.Error("had = false, want true: a baseline directory exists")
	}
	if cleared != 2 {
		t.Errorf("cleared = %d, want 2: a.example was in p1 and p2, not p3", cleared)
	}
}

// TestForgetAll_NoBaselineDirectoryIsNotAnError pins the behaviour
// clearBaselines relied on: a store with no baseline directory yet reports
// hadBaselines=false and no error, not ErrNotAHostname's sibling or a lock
// failure on a directory that was never created.
func TestForgetAll_NoBaselineDirectoryIsNotAnError(t *testing.T) {
	cleared, had, err := ForgetAll(filepath.Join(t.TempDir(), "baseline"), "a.example")
	if err != nil || had || cleared != 0 {
		t.Errorf("ForgetAll on a store with no baseline directory: cleared=%d had=%v err=%v, want 0, false, nil",
			cleared, had, err)
	}
}
