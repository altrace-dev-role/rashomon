package regfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// Open opens a regular file, refuses a directory and leaves a missing path
// as the open's own error, on every platform the Test job runs: none of the
// three needs a named pipe (regfile_fifo_test.go has that row).
func TestSRR1_OpenRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path string
		want       error // nil: opens
	}{
		{"directory", dir, ErrNotRegular},
		{"regular file", file, nil},
		{"missing", filepath.Join(dir, "none"), fs.ErrNotExist},
	} {
		f, err := Open(c.path)
		if f != nil {
			f.Close() //nolint:errcheck // read-only
		}
		switch {
		case c.want == nil && err != nil:
			t.Errorf("%s: Open = %v, want it open", c.name, err)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: Open = %v, want %v", c.name, err, c.want)
		case c.want != nil && f != nil:
			t.Errorf("%s: Open returned a file with error %v", c.name, err)
		}
	}
}
