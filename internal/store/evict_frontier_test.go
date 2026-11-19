package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEvict_RemovesTheFrontierWithItsRun: eviction removes a run's directory
// whole, so the run's frontier and a temporary file a save left go with its
// records, and no frontier outlives the lines it was folded from. A run the
// pass keeps keeps its frontier.
func TestEvict_RemovesTheFrontierWithItsRun(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const oldSession, keptSession = "sess-old", "sess-kept"
	for _, id := range []string{oldSession, keptSession} {
		if err := st.AppendDeclaration(testDecl(id, "toolu_"+id)); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveFrontier(id, []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	oldTemp := filepath.Join(st.RunDir(oldSession), "."+FileFrontier+".tmp-1")
	if err := os.WriteFile(oldTemp, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := chtimesTree(st.RunDir(oldSession), time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Evict(1, "", time.Now()); err != nil {
		t.Fatalf("Evict: %v", err)
	}
	for _, p := range []string{filepath.Join(st.RunDir(oldSession), FileFrontier), oldTemp} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s is left after its run was evicted: %v", filepath.Base(p), err)
		}
	}
	if _, ok, err := st.LoadFrontier(keptSession); !ok || err != nil {
		t.Errorf("the kept run's frontier is gone (ok %v, err %v)", ok, err)
	}
}
