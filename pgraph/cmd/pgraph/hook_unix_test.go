//go:build unix

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPW1b_ASyncWaitsForTheLockAndWritesNothingUnderIt (review item 2 on
// #230): two syncs of one root at once can rename A's log, then B's log and
// digests, then A's digests, leaving digests that vouch for a log they were
// not written with. So every sync, a person's or the hook's, holds a lock on
// the root while it reads and writes, and one that cannot take it before its
// timeout writes nothing: the person's exits 1 saying so, the hook's exits 0
// with that line on stderr. The lock is on the root directory itself, so a
// sync that fails still leaves no .pgraph. Break: take no lock, and a sync
// writes under another's.
func TestPW1b_ASyncWaitsForTheLockAndWritesNothingUnderIt(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	root := repo(t, smallModule)
	unlock, err := lockRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	code, _, errs := sync(root, "-timeout", "300ms")
	if code != 1 || !strings.Contains(errs, "another sync") {
		t.Errorf("a person's sync under a held lock: exit %d, stderr %q; want 1 naming the other sync", code, errs)
	}
	code, out, errs := hookRun(t, payload(root), "-timeout", "300ms")
	if code != 0 || out != "" || !strings.Contains(errs, "another sync") {
		t.Errorf("the hook under a held lock: exit %d, %q %q; want 0 and the line on stderr", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(root, ".pgraph")); err == nil {
		t.Error("a sync wrote .pgraph under another's lock")
	}
	unlock()
	if code, _, errs := sync(root, "-timeout", "30s"); code != 0 {
		t.Errorf("after the lock is released: exit %d: %s", code, errs)
	}
}
