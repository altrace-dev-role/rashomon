//go:build unix

package index

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// TestWire_AnUnreadableGoModRefusesGo: a go.mod at mode 000 is there and
// cannot be opened, a read that fails: Go under that root is refused with
// the open's error, naming go.mod, fs.ErrPermission kept. Unix only: on
// windows mode 000 leaves the file readable. Root opens any file, so the
// test skips as root. Break: let the Go step drop the error.
func TestWire_AnUnreadableGoModRefusesGo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a file at mode 000")
	}
	root := wireRoot(t, map[string]string{"go.mod": "module example.com/m\n"})
	if err := os.Chmod(filepath.Join(root, "go.mod"), 0); err != nil {
		t.Fatal(err)
	}
	r, ok, err := Wire().Claim(root, "a/a.go")
	if !ok || r != nil || !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("Claim(a/a.go) = %v, %v, %v; want nil and an error naming go.mod, fs.ErrPermission kept", r, ok, err)
	}
}

// TestWire_AnUnreadableFileIsAFailedRead: a configuration or a go.mod at
// mode 000 is refused as a failed read, the reason no other one names, the
// refusal naming its own file. Root opens any file, so the test skips as
// root. Break: read every refusal as a file that is not regular.
func TestWire_AnUnreadableFileIsAFailedRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a file at mode 000")
	}
	for _, c := range []struct{ file, rel string }{{ConfigFile, "inv/core/calc.py"}, {gopg.ManifestFile, "a/a.go"}} {
		root := wireRoot(t, map[string]string{c.file: "{}\n"})
		if err := os.Chmod(filepath.Join(root, filepath.FromSlash(c.file)), 0); err != nil {
			t.Fatal(err)
		}
		if rf := wireRefusal(t, root, c.rel); rf.File != c.file || rf.Reason != lang.ReadFailed {
			t.Errorf("%s at mode 000: the refusal names %q, reason %d; want %q, a failed read", c.file, rf.File, rf.Reason, c.file)
		}
	}
}
