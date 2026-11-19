//go:build unix

package index

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
