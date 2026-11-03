package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/seed"
)

// TestSEED2d_PurgeTakesTheKeptCount: the kept before count `rashomon audit
// --keep` writes, and the temporary file its replace leaves, are rashomon's
// and gone after purge (PF-HD10's zero residue); a user's file named like it
// is foreign and stays.
func TestSEED2d_PurgeTakesTheKeptCount(t *testing.T) {
	root := storeAt(t)
	ours := []string{seed.KeptFile, "." + seed.KeptFile + ".tmp-1"}
	mine := seed.KeptFile + ".bak"
	for _, f := range append(slices.Clone(ours), mine) {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ours {
		if !slices.Contains(p.Files, filepath.Join(root, f)) {
			t.Errorf("the plan does not list %s: %q", f, p.Files)
		}
	}
	if abs := filepath.Join(root, mine); !slices.Contains(p.Foreign, abs) || slices.Contains(p.Files, abs) {
		t.Errorf("the plan does not name %s as foreign, or lists it as rashomon's: foreign %q", mine, p.Foreign)
	}
	p.Execute()
	for _, f := range ours {
		if _, err := os.Lstat(filepath.Join(root, f)); err == nil {
			t.Errorf("purge left %s", f)
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, mine)); err != nil || string(b) != "x" {
		t.Errorf("purge removed %s (%v)", mine, err)
	}
}
