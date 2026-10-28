package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRC14d_PurgeLeavesNoFile: transcript copies, the switch and the
// temporary files their writes leave are rashomon's, listed deepest first and
// gone after purge; a user's file beside the project directories is foreign
// and stays (card 4.1, Gate B 1: a third branch in treeFile).
func TestRC14d_PurgeLeavesNoFile(t *testing.T) {
	root := storeAt(t)
	ours := []string{
		store.FileBodies, "." + store.FileBodies + ".tmp-1",
		"bodies/work_a-0123456789ab/s1.jsonl", "bodies/work_a-0123456789ab/" + store.BodiesTempPrefix + "7",
		"bodies/work_b-ba9876543210/s2.jsonl",
	}
	mine := "bodies/notes.txt"
	for _, f := range append(slices.Clone(ours), mine) {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	at := func(f string) int { return slices.Index(p.Files, filepath.Join(root, filepath.FromSlash(f))) }
	for _, f := range ours {
		if at(f) < 0 {
			t.Errorf("the plan does not list %s: %q", f, p.Files)
		}
	}
	if f, d := at("bodies/work_a-0123456789ab/s1.jsonl"), at("bodies/work_a-0123456789ab"); d < 0 || f > d {
		t.Errorf("the project directory is not listed after its copies (file %d, directory %d)", f, d)
	}
	abs := filepath.Join(root, filepath.FromSlash(mine))
	if !slices.Contains(p.Foreign, abs) || at(mine) >= 0 {
		t.Errorf("the plan does not name %s as foreign, or lists it as rashomon's: foreign %q", mine, p.Foreign)
	}
	p.Execute()
	for _, f := range ours {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			t.Errorf("purge left %s", f)
		}
	}
	if b, err := os.ReadFile(abs); err != nil || string(b) != "x" {
		t.Errorf("purge removed %s (%v)", mine, err)
	}
}
