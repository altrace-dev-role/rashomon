//go:build unix

package rootio

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestWS1_RF5_ReplaceFileCreatesWithPermUnderTheUmask: under umask 022 a
// file replaced with perm 0o666 reads mode 0644 and one replaced with 0o600
// reads 0600, each over an old file of mode 0640: the mode is the new
// create's under the umask, not the old file's, and nothing changes it
// after. The umask is the process's; no test in this package runs in
// parallel. Break: create the temporary file 0600, or 0666, whatever perm
// says.
func TestWS1_RF5_ReplaceFileCreatesWithPermUnderTheUmask(t *testing.T) {
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
	root := t.TempDir()
	r := openRoot(t, root)
	p := filepath.Join(root, "f")
	for _, c := range []struct{ perm, want fs.FileMode }{{0o666, 0o644}, {0o600, 0o600}} {
		if err := os.WriteFile(p, oldBytes, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o640); err != nil {
			t.Fatal(err)
		}
		err := ReplaceFile(r, "f", c.perm, roomy, func(w io.Writer) error {
			_, err := w.Write([]byte("the new file\n"))
			return err
		})
		if err != nil {
			t.Fatalf("ReplaceFile with perm %#o: %v", c.perm, err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != c.want {
			t.Errorf("perm %#o under umask 022: f reads mode %#o; want %#o", c.perm, got, c.want)
		}
	}
}
