//go:build unix

package graph

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestWS1_SF4_SaveFileCreatesTheIndexWithMode0600: under umask 022 the
// saved index reads mode 0600, the mode of the temporary file it replaces,
// also where the index it replaces read 0644. The umask is the process's;
// no test in this package runs in parallel. Break: save the index with perm
// 0o644.
func TestWS1_SF4_SaveFileCreatesTheIndexWithMode0600(t *testing.T) {
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
	root := t.TempDir()
	p := filepath.Join(root, filepath.FromSlash(GraphFile))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(root, NewStore(), Header{Repo: "r", Rev: HeaderRev{Commit: "C1"}}); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("%s under umask 022 reads mode %#o; want 0600", p, got)
	}
}
