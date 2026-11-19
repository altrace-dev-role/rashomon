package rootio

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// oldBytes is what name holds before a replace these tests make fail.
var oldBytes = []byte("the file as it was before the replace\n")

// errWrite is the failure a test's write function returns.
var errWrite = errors.New("the write failed part way")

// openRoot opens dir as a root, closed when the test ends.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() }) //nolint:errcheck // the root holds only a directory handle
	return r
}

// entries is the names dir holds.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// link makes at a symlink to target, or skips the test: Windows creates one
// only with the privilege or developer mode.
func link(t *testing.T, target, at string) {
	t.Helper()
	if err := os.Symlink(target, at); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
}

// TestWS1_RF6_ReplaceFileLeavesNameAsItWasWhenAStepFails: with name already
// there, a write that fails after putting bytes, and a rename refused
// because a directory holds name's place, each return an error, leave what
// was at name as it was, and leave nothing beside it: no .<base>.tmp-* file.
// A refused MkdirAll is TestWS1_I1_'s and TestWS1_I10b_'s; a close of a file
// on a local disk does not fail, so no test reaches that step. Break: keep
// the temporary file after a failed write, or after a failed rename, or
// rename a failed write's file over name.
func TestWS1_RF6_ReplaceFileLeavesNameAsItWasWhenAStepFails(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "f"), oldBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		err := ReplaceFile(openRoot(t, root), filepath.Join("sub", "f"), 0o644, func(w io.Writer) error {
			if _, err := w.Write([]byte("the new file's first half")); err != nil {
				return err
			}
			return errWrite
		})
		if !errors.Is(err, errWrite) {
			t.Errorf("ReplaceFile = %v; want the write's own error", err)
		}
		if got, err := os.ReadFile(filepath.Join(sub, "f")); err != nil || !bytes.Equal(got, oldBytes) {
			t.Errorf("sub/f after the failed write reads %q (err %v); want %q", got, err, oldBytes)
		}
		if got := entries(t, sub); len(got) != 1 || got[0] != "f" {
			t.Errorf("sub holds %v after the failed write; want only f", got)
		}
	})
	t.Run("rename", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "sub")
		if err := os.MkdirAll(filepath.Join(sub, "d"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "d", "keep"), oldBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		err := ReplaceFile(openRoot(t, root), filepath.Join("sub", "d"), 0o644, func(w io.Writer) error {
			_, err := w.Write([]byte("a file no rename can put in a directory's place"))
			return err
		})
		if err == nil {
			t.Error("ReplaceFile over a directory holding a file returned no error")
		}
		if got, err := os.ReadFile(filepath.Join(sub, "d", "keep")); err != nil || !bytes.Equal(got, oldBytes) {
			t.Errorf("sub/d/keep after the failed rename reads %q (err %v); want %q", got, err, oldBytes)
		}
		if got := entries(t, sub); len(got) != 1 || got[0] != "d" {
			t.Errorf("sub holds %v after the failed rename; want only d", got)
		}
	})
}

// TestWS1_RF7_ReplaceFileErrorNamesTheWholeTarget: a failed replace's error
// is one line that begins with the root's path joined with name, not name
// alone. Break: name only name in the error.
func TestWS1_RF7_ReplaceFileErrorNamesTheWholeTarget(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join("a", "b", "f.json")
	err := ReplaceFile(openRoot(t, root), name, 0o644, func(io.Writer) error { return errWrite })
	want := filepath.Join(root, name) + ": "
	switch {
	case err == nil:
		t.Fatal("ReplaceFile returned no error for a failed write")
	case !strings.HasPrefix(err.Error(), want):
		t.Errorf("ReplaceFile = %q; want it to begin %q", err, want)
	case strings.Contains(err.Error(), "\n"):
		t.Errorf("ReplaceFile = %q; want one line", err)
	}
}

// TestWS1_RR5_ReadRegularErrorsNameTheRootJoinedWithName: a missing source,
// a directory where a source was named and a source over the limit each
// return no byte and an error naming the root's path joined with name, not
// the base name alone; the directory's is the reader's own *fs.PathError of
// that path wrapping regfile.ErrNotRegular, and the over-limit one is the
// reader's own *fs.PathError of that path wrapping a *regfile.OverCapError
// of the size and the limit, the form a caller records by its class without
// the path; both are returned as they are. Break: name the error by name
// alone; or wrap the not-regular refusal again, or the over-limit refusal,
// so its text names the path twice.
func TestWS1_RR5_ReadRegularErrorsNameTheRootJoinedWithName(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(filepath.Join(app, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "big.py"), bytes.Repeat([]byte("x"), 65), 0o644); err != nil {
		t.Fatal(err)
	}
	r := openRoot(t, root)
	for _, c := range []struct {
		name string
		also string // a further text the error must hold
		dir  bool   // a *fs.PathError wrapping regfile.ErrNotRegular
		over bool   // a *fs.PathError of the joined path wrapping a *regfile.OverCapError
	}{
		{filepath.Join("app", "none.py"), "", false, false},
		{filepath.Join("app", "pkg"), "", true, false},
		{filepath.Join("app", "big.py"), "65 bytes, over the 64 byte cap", false, true},
	} {
		b, err := ReadRegular(r, c.name, 64)
		want := filepath.Join(root, c.name)
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), c.also) {
			t.Errorf("ReadRegular(%s) = %v; want an error naming %s %s", c.name, err, want, c.also)
		}
		if b != nil {
			t.Errorf("ReadRegular(%s) returned %d bytes with its error", c.name, len(b))
		}
		pe, ok := err.(*fs.PathError)
		if c.dir && (!ok || pe.Path != want || !errors.Is(err, regfile.ErrNotRegular)) {
			t.Errorf("ReadRegular(%s) = %#v; want the reader's *fs.PathError of %s wrapping regfile.ErrNotRegular, as it is",
				c.name, err, want)
		}
		var oc *regfile.OverCapError
		if c.over && (!ok || pe.Path != want || !errors.As(err, &oc) || oc.Size != 65 || oc.Max != 64 ||
			!errors.Is(err, regfile.ErrOverCap)) {
			t.Errorf("ReadRegular(%s) = %#v; want the reader's *fs.PathError of %s wrapping a *regfile.OverCapError of 65 bytes over 64, as it is",
				c.name, err, want)
		}
	}
}

// TestWS1_PK1_ReadRegularRefusesLinksOutOfTheRootAndAbsoluteOnes: a source
// under a directory that is a relative link climbing out of the root, and a
// source that is an absolute link even to a file inside the root, are
// refused with no byte and an error naming the root's path joined with
// name; a relative link to a file inside is read, as an open by path read
// it. Break: open by the joined path, which follows every link.
func TestWS1_PK1_ReadRegularRefusesLinksOutOfTheRootAndAbsoluteOnes(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "x.py"), []byte("outside = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inside := []byte("inside = 1\n")
	if err := os.WriteFile(filepath.Join(root, "in.py"), inside, 0o644); err != nil {
		t.Fatal(err)
	}
	up, err := filepath.Rel(root, out)
	if err != nil {
		t.Fatal(err)
	}
	link(t, up, filepath.Join(root, "app"))
	link(t, filepath.Join(root, "in.py"), filepath.Join(root, "abs.py"))
	link(t, "in.py", filepath.Join(root, "rel.py"))
	r := openRoot(t, root)
	for _, name := range []string{filepath.Join("app", "x.py"), "abs.py"} {
		b, err := ReadRegular(r, name, 1<<10)
		want := filepath.Join(root, name)
		if err == nil || b != nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ReadRegular(%s) = %q, %v; want no byte and an error naming %s", name, b, err, want)
		}
	}
	if b, err := ReadRegular(r, "rel.py", 1<<10); err != nil || !bytes.Equal(b, inside) {
		t.Errorf("ReadRegular(rel.py), a relative link inside the root, = %q, %v; want %q", b, err, inside)
	}
}
