package store

import (
	"os"
	"testing"
	"time"
)

// TestEvict_EmptyRunDirIsNotTreatedAsTheOldest: sizeRun took a run's mtime
// only from its files, so a run directory with none yet -- created by
// MkdirAll an instant before its first file is opened (store.go's
// appendLine) -- reported the zero time, which sorts before every real
// run's mtime and reads as far older than evictGrace allows. A session
// that has only just started, caught in that window, must not be evicted
// as though it were the store's oldest run.
func TestEvict_EmptyRunDirIsNotTreatedAsTheOldest(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const oldSession, freshSession = "sess-old", "sess-justmade"
	if err := st.AppendDeclaration(testDecl(oldSession, "toolu_old")); err != nil {
		t.Fatal(err)
	}
	if err := chtimesTree(st.RunDir(oldSession), time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.RunDir(freshSession), dirMode); err != nil {
		t.Fatal(err)
	}

	gaps, err := st.Evict(1, "", time.Now())
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if _, serr := os.Lstat(st.RunDir(freshSession)); serr != nil {
		t.Errorf("the freshly created empty run was removed: %v, want it left alone inside evictGrace", serr)
	}
	if _, serr := os.Lstat(st.RunDir(oldSession)); !os.IsNotExist(serr) {
		t.Errorf("sess-old still exists after eviction: %v", serr)
	}
	for _, g := range gaps {
		if g.SessionID == freshSession {
			t.Errorf("a gap was written for the run that should not have been evicted: %+v", g)
		}
	}
}
