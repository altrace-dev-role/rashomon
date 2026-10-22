//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEvictSkipsARunARefusedReadHolds: a run eviction cannot read because
// another handle's lock refuses it (Windows's ERROR_LOCK_VIOLATION) is a
// live session, not a damaged one, and is left for the next pass rather
// than removed. An ordinary read error on an old run is still removed, by
// size, as it always was.
func TestEvictSkipsARunARefusedReadHolds(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sess-refused", "sess-old"} {
		if err := st.AppendDeclaration(testDecl(s, "toolu_"+s)); err != nil {
			t.Fatal(err)
		}
	}
	refusedDir := st.RunDir("sess-refused")
	recordsPath := filepath.Join(refusedDir, FileRecords)
	if err := os.Remove(recordsPath); err != nil {
		t.Fatal(err)
	}
	release := refuseRecordsRead(t, st, recordsPath)
	defer release()

	old := time.Now().Add(-48 * time.Hour)
	for _, s := range []string{"sess-refused", "sess-old"} {
		if err := chtimesTree(st.RunDir(s), old); err != nil {
			t.Fatal(err)
		}
	}

	gaps, err := st.Evict(1, "", time.Now())
	if err != nil {
		t.Fatalf("Evict with a refused run: %v, want the refusal skipped rather than returned", err)
	}
	if _, serr := os.Lstat(refusedDir); serr != nil {
		t.Errorf("a run whose read was refused was removed: %v", serr)
	}
	if _, serr := os.Lstat(st.RunDir("sess-old")); !os.IsNotExist(serr) {
		t.Errorf("sess-old still exists after eviction: %v", serr)
	}
	for _, g := range gaps {
		if g.SessionID == "sess-refused" {
			t.Errorf("a gap was written for the run eviction skipped: %+v", g)
		}
	}
}
