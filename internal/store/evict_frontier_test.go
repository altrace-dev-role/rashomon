package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEvict_RemovesTheFrontierWithItsRun: eviction removes a run's directory
// whole, so the run's frontier and a temporary file a save left go with its
// records: once a pass has removed a run, no frontier of it is left. A save
// that races the removal is the limit stated at ComposeFrontier. A run the
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

// TestEvict_ARunThatHoldsOnlyItsFrontierGoesOnTheNextPass: eviction takes
// no run's records lock, so a save can land in a run directory while a pass
// removes it, and the directory can be left holding frontier.json alone;
// that pass's removal fails, and its error is Evict's, which probe prints
// as its fixed eviction line on stderr. To the passes after it the
// directory is a run like any other, dated by its newest file: a pass
// removes nothing while the store fits under its limit, stops once it
// fits or the hook's deadline passes, and skips a run written within
// evictGrace. So the first pass that still needs room when it reaches the
// directory, an hour or more after the save, removes it whole, with a gap
// that counts no records; in a store that stays under its limit the file
// stays. It is at most MaxFrontierBytes and is never continued (see
// ComposeFrontier). The limit of 1 byte below makes the pass need room.
func TestEvict_ARunThatHoldsOnlyItsFrontierGoesOnTheNextPass(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const leftSession = "sess-left"
	if err := st.AppendDeclaration(testDecl(leftSession, "toolu_"+leftSession)); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveFrontier(leftSession, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	dir := st.RunDir(leftSession)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != FileFrontier {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	if left, err := os.ReadDir(dir); err != nil || len(left) != 1 || left[0].Name() != FileFrontier {
		t.Fatalf("the run directory should hold only %s before the pass: %v, %v", FileFrontier, left, err)
	}
	if err := chtimesTree(dir, time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	gaps, err := st.Evict(1, "", time.Now())
	if err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("the run directory that held only its frontier is left after the pass: %v", err)
	}
	if len(gaps) != 1 || gaps[0].RemovedRecords != 0 {
		t.Errorf("gaps = %+v, want one gap that counts no records", gaps)
	}
}
