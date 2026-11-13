package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestPurge_RemovesTheFrontierAndItsTemp: the frontier the store saves in a
// run directory, and the temporary name its atomic write leaves when a
// process dies between writing and renaming, are rashomon's. The plan lists
// both before their run directory, nothing is foreign, and after the plan
// runs neither is on disk.
func TestPurge_RemovesTheFrontierAndItsTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rashomon")
	ours := []string{"runs/s1/" + store.FileFrontier, "runs/s1/." + store.FileFrontier + ".tmp-x"}
	for _, f := range append([]string{InstallMeta, InstallKey, "runs/s1/" + store.FileRecords}, ours...) {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	at := func(f string) int { return slices.Index(p.Files, filepath.Join(root, filepath.FromSlash(f))) }
	dir := at("runs/s1")
	for _, f := range ours {
		if i := at(f); i < 0 || dir < 0 || i > dir {
			t.Errorf("the plan does not list %s before its run directory (file %d, directory %d)", f, i, dir)
		}
	}
	if len(p.Foreign) != 0 || !p.RemoveRoot {
		t.Errorf("foreign %v, RemoveRoot %v: want nothing foreign and the root removed", p.Foreign, p.RemoveRoot)
	}
	p.Execute()
	for _, f := range ours {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			t.Errorf("purge left %s, which rashomon wrote", f)
		}
	}
}
