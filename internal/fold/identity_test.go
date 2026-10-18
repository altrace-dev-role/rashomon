package fold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentity_SameFileIsEqual(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.ndjson")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dev1, inode1, exists1, err := identity(path)
	if err != nil {
		t.Fatal(err)
	}
	if !exists1 {
		t.Fatal("exists = false for a file that is there")
	}
	dev2, inode2, exists2, err := identity(path)
	if err != nil {
		t.Fatal(err)
	}
	if !exists2 || dev1 != dev2 || inode1 != inode2 {
		t.Errorf("identity read twice disagreed: (%d,%d,%v) vs (%d,%d,%v)", dev1, inode1, exists1, dev2, inode2, exists2)
	}
}

func TestIdentity_RewriteChangesInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.ndjson")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, before, _, err := identity(path)
	if err != nil {
		t.Fatal(err)
	}

	// A rewrite (forget/evict) removes the old inode and creates a new one
	// at the same path; a plain append must never look like this.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, after, _, err := identity(path)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Errorf("inode %d unchanged after the file was removed and recreated", before)
	}
}

func TestIdentity_MissingFileReportsNotExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.ndjson")
	_, _, exists, err := identity(path)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("exists = true for a file that was never created")
	}
}
