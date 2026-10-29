//go:build unix

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSC1b_AFailedWriteLeavesTheMarkerWhole: a write that cannot complete,
// here because the root has turned read-only, leaves the marker that was
// there, whole: the reader finds the old value or the new one, never a part.
func TestSC1b_AFailedWriteLeavesTheMarkerWhole(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	root := t.TempDir()
	if err := WriteSteerDisclosureMarker(root, 2); err != nil {
		t.Fatalf("WriteSteerDisclosureMarker(2): %v", err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if err := WriteSteerDisclosureMarker(root, 5); err == nil {
		t.Fatalf("a write into a read-only root succeeded; the premise does not hold")
	}
	b, err := os.ReadFile(filepath.Join(root, FileSteerDisclosure))
	if err != nil || string(b) != "2\n" {
		t.Errorf("after a failed write the marker holds %q (%v), want \"2\\n\" whole", b, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("a failed write left %d entries, want the marker alone", len(entries))
	}
}
