package store

import (
	"os"
	"testing"
	"time"
)

// TestEvictStopsAtItsCallersDeadline: eviction runs inside a hook probe on a
// shared deadline (LimitLockWaits). A backlog of old runs must not make one
// pass outlive that deadline looking for more to remove -- it stops before
// starting work on the next candidate, and leaves it for the next probe,
// rather than risk being killed partway through a removal.
func TestEvictStopsAtItsCallersDeadline(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessions := []string{"sess-a", "sess-b", "sess-c"}
	for _, s := range sessions {
		if err := st.AppendDeclaration(testDecl(s, "toolu_"+s)); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	for _, s := range sessions {
		if err := chtimesTree(st.RunDir(s), old); err != nil {
			t.Fatal(err)
		}
	}

	st.LimitLockWaits(0) // a deadline already in the past when Evict starts
	gaps, err := st.Evict(1, "", time.Now())
	if err != nil {
		t.Fatalf("Evict past its deadline: %v, want it to stop cleanly instead", err)
	}
	if len(gaps) != 0 {
		t.Errorf("Evict past its deadline removed %d runs, want it to stop before the first", len(gaps))
	}
	for _, s := range sessions {
		if _, serr := os.Lstat(st.RunDir(s)); serr != nil {
			t.Errorf("run %s missing after an Evict that should have done nothing: %v", s, serr)
		}
	}
}
