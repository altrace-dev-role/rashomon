//go:build unix

package knowledge

import (
	"os"
	"syscall"
	"testing"
)

// TestWS1_RF5k_WriteFileCreatesTheRowFileWithMode0644: under umask 022 a
// written row file reads mode 0644, the mode its write by path gave it. The
// umask is the process's; no test in this package runs in parallel. Break:
// write the row file with perm 0o600.
func TestWS1_RF5k_WriteFileCreatesTheRowFileWithMode0644(t *testing.T) {
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
	p, err := WriteFile(t.TempDir(), "x", nil)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("%s under umask 022 reads mode %#o; want 0644", p, got)
	}
}
