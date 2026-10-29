package disclose

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestTRU23_BodiesPathsAreInTheInventory: copies are off by default, so
// TestTRU23_EveryWrittenPathIsInTheInventory never reaches them. Here the
// switch is on and a copy is taken, and the switch, the copy, the temporary
// files a crash in either write leaves, and anything else under bodies/ are
// described by Default().Files (card 4.1).
func TestTRU23_BodiesPathsAreInTheInventory(t *testing.T) {
	const key, dir, session = "/work/proj", "work_proj-0123456789ab", "sess-bodies"
	root := t.TempDir()
	s, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteBodies(root, store.BodiesSetting{MaxBytes: 1 << 20, Projects: []string{key}}); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(src, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if wrote, err := s.CopyBody(session, src, key, dir, time.Now()); !wrote || err != nil {
		t.Fatalf("CopyBody = (%v, %v)", wrote, err)
	}
	crashed := []string{"." + store.FileBodies + ".tmp-1", filepath.Join(store.DirBodies, dir, store.BodiesTempPrefix+"1")}
	for _, rel := range crashed {
		if err := os.WriteFile(filepath.Join(root, rel), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range append(crashed, store.FileBodies, filepath.Join(store.DirBodies, dir, s.DirName(session)+".jsonl")) {
		if _, err := os.Lstat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s was not written: %v", rel, err)
		} else if !matchesAFileRule(rel, inv.Files) {
			t.Errorf("store wrote %q, which Default().Files does not cover", rel)
		}
	}
	err = filepath.WalkDir(filepath.Join(root, store.DirBodies), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err == nil && !matchesAFileRule(rel, inv.Files) {
			t.Errorf("store wrote %q, which Default().Files does not cover", rel)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
