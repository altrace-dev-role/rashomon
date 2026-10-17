//go:build unix

package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestTRU28_AnEmptyCreatedDirThatCannotGoIsStranded: a created directory
// that is empty and still cannot be removed -- its parent is not writable
// -- is a planned path that remains, not one left for being in use.
func TestTRU28_AnEmptyCreatedDirThatCannotGoIsStranded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes from a directory it cannot write")
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "rashomon")
	parent := filepath.Join(dir, "locked")
	created := filepath.Join(parent, "made")
	writeFiles(t, filepath.Join(root, InstallMeta), filepath.Join(root, InstallKey))
	if err := os.MkdirAll(created, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o700) }) //nolint:errcheck // TempDir cleanup reports it

	p, err := Build(root, []string{created, root}, "")
	if err != nil {
		t.Fatal(err)
	}
	out := p.Execute()
	if slices.Contains(out.Left, created) {
		t.Errorf("Left = %q; an empty directory is not left for being in use", out.Left)
	}
	if !slices.Equal(out.Stranded, []string{created}) {
		t.Errorf("Stranded = %q, want %s", out.Stranded, created)
	}
}
