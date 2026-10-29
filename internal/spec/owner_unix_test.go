//go:build unix

package spec

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestVER41b_OwnerIsLstats: OSFS.Owner reads a file's owner and mode, and a
// symlink's as a symlink, never its target's, so a link to a file passes
// adminOwned for neither.
func TestVER41b_OwnerIsLstats(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(file, []byte(docA), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	uid, mode, err := (OSFS{}).Owner(file)
	if err != nil || uid != os.Getuid() || !mode.IsRegular() {
		t.Errorf("a file: uid %d, mode %s, %v; want %d, a regular file", uid, mode, err, os.Getuid())
	}
	if uid, mode, err := (OSFS{}).Owner(dir); err != nil || uid != os.Getuid() || !mode.IsDir() {
		t.Errorf("a directory: uid %d, mode %s, %v", uid, mode, err)
	}
	if _, mode, err := (OSFS{}).Owner(link); err != nil || mode&fs.ModeSymlink == 0 {
		t.Errorf("a symlink: mode %s, %v; want a symlink", mode, err)
	}
	if uid, _, err := (OSFS{}).Owner(filepath.Join(dir, "none")); !os.IsNotExist(err) || uid != -1 {
		t.Errorf("a missing path: uid %d, %v; want -1 and not-exist", uid, err)
	}
}
