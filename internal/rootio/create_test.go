package rootio

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// setupName is the path the setup writes under a repository's root.
var setupName = filepath.Join(".rashomon", "graph.json")

// newBytes is what a test's CreateFile writes.
var newBytes = []byte(`{"units":[]}` + "\n")

// writeNew is a write func that puts newBytes.
func writeNew(w io.Writer) error {
	_, err := w.Write(newBytes)
	return err
}

// noTemporary fails the test when dir holds a temporary of createTemp's
// form.
func noTemporary(t *testing.T, dir string) {
	t.Helper()
	for _, n := range entries(t, dir) {
		if strings.Contains(n, ".tmp-") {
			t.Errorf("%s holds the temporary %s; want it removed", dir, n)
		}
	}
}

// absent fails the test when anything, a link included, is at p.
func absent(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat %s = %v; want nothing there", p, err)
	}
}

// TestSETUP1_CF1_CreateFileWritesWhereNothingIs: with nothing at name, the
// parents are made and name holds the bytes written, with no temporary
// beside it.
func TestSETUP1_CF1_CreateFileWritesWhereNothingIs(t *testing.T) {
	dir := t.TempDir()
	if err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew); err != nil {
		t.Fatalf("CreateFile = %v; want the file written", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, setupName))
	if err != nil || string(got) != string(newBytes) {
		t.Fatalf("name holds %q, %v; want %q", got, err, newBytes)
	}
	if got := entries(t, filepath.Join(dir, ".rashomon")); !slices.Equal(got, []string{"graph.json"}) {
		t.Errorf(".rashomon holds %v; want only graph.json", got)
	}
}

// TestSETUP1_CF2_CreateFileLeavesAFileThereAndNoTemporary (a1): a file at
// name is refused with an error matching fs.ErrExist that names the whole
// target; its bytes and its inode are as they were, and no temporary is
// left beside it.
func TestSETUP1_CF2_CreateFileLeavesAFileThereAndNoTemporary(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, setupName)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, oldBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	err = CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("CreateFile over a file = %v; want an error matching fs.ErrExist", err)
	}
	if !strings.HasPrefix(err.Error(), p+": ") || strings.Contains(err.Error(), "\n") {
		t.Errorf("error %q; want one line starting %q", err, p+": ")
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != string(oldBytes) {
		t.Errorf("name holds %q, %v; want its old bytes", got, err)
	}
	if after, err := os.Stat(p); err != nil || !os.SameFile(before, after) {
		t.Errorf("name is not the same file after the refusal (%v)", err)
	}
	noTemporary(t, filepath.Dir(p))
}

// TestSETUP1_CF3_CreateFileLeavesADirectoryThere: a directory at name is
// refused with an error matching fs.ErrExist, and holds what it held.
func TestSETUP1_CF3_CreateFileLeavesADirectoryThere(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, setupName)
	if err := os.MkdirAll(filepath.Join(p, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("CreateFile over a directory = %v; want an error matching fs.ErrExist", err)
	}
	if got := entries(t, p); !slices.Equal(got, []string{"inside"}) {
		t.Errorf("the directory holds %v; want [inside]", got)
	}
	noTemporary(t, filepath.Dir(p))
}

// errRefused is the failure a test's link returns.
var errRefused = errors.New("this filesystem makes no hard link")

// TestSETUP1_CF4_ALinkThatFailsWritesNothing (a2): when the link is refused,
// CreateFile returns that error, tries the link once and nothing else, and
// the root lists what it listed before: nothing at name, no temporary, and
// no parent left that the create made.
func TestSETUP1_CF4_ALinkThatFailsWritesNothing(t *testing.T) {
	calls := 0
	was := publish
	publish = func(*os.Root, string, string) error { calls++; return errRefused }
	t.Cleanup(func() { publish = was })
	dir := t.TempDir()
	err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew)
	if !errors.Is(err, errRefused) {
		t.Fatalf("CreateFile with the link refused = %v; want that refusal", err)
	}
	if calls != 1 {
		t.Errorf("the link was tried %d time(s); want once", calls)
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Errorf("the root lists %v after the refusal; want what it listed before, nothing", got)
	}
}

// TestSETUP1_CF5_CreateFileRefusesAWriteOverMax: a write over max is
// refused by the cap's error, after the capped writer has put bytes in the
// temporary, and the root lists what it listed before.
func TestSETUP1_CF5_CreateFileRefusesAWriteOverMax(t *testing.T) {
	dir := t.TempDir()
	err := CreateFile(openRoot(t, dir), setupName, 0o644, int64(len(newBytes)-1), writeNew)
	if !errors.Is(err, regfile.ErrOverCap) {
		t.Fatalf("CreateFile over max = %v; want regfile.ErrOverCap", err)
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Errorf("the root lists %v after the refusal; want what it listed before, nothing", got)
	}
}

// TestSETUP1_CF6_AFailedWriteLeavesNothing: write's own error is returned,
// and the root lists what it listed before.
func TestSETUP1_CF6_AFailedWriteLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, func(io.Writer) error { return errWrite })
	if !errors.Is(err, errWrite) {
		t.Fatalf("CreateFile = %v; want the write's error", err)
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Errorf("the root lists %v after the refusal; want what it listed before, nothing", got)
	}
}

// TestSETUP1_CF7_CreateFileLeavesALinkAsItIs: a link at name, to a file in
// the root, to one outside it, or to nothing, is refused with an error
// matching fs.ErrExist; the link reads as it did, its target holds what it
// held, and nothing is made where a dangling link points.
func TestSETUP1_CF7_CreateFileLeavesALinkAsItIs(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, oldBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, target string
		made         bool // the test makes the target, inside the root
	}{
		{"to a file in the root", "kept.json", true},
		{"to a file outside the root", outside, false},
		{"to nothing", "missing.json", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, setupName)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if c.made {
				if err := os.WriteFile(filepath.Join(dir, ".rashomon", c.target), oldBytes, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			link(t, c.target, p)
			err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew)
			if !errors.Is(err, fs.ErrExist) {
				t.Fatalf("CreateFile over a link %s = %v; want an error matching fs.ErrExist", c.name, err)
			}
			if got, err := os.Readlink(p); err != nil || got != c.target {
				t.Errorf("the link reads %q, %v; want %q", got, err, c.target)
			}
			target := c.target
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, ".rashomon", target)
			}
			switch got, err := os.ReadFile(target); {
			case c.made || target == outside:
				if err != nil || string(got) != string(oldBytes) {
					t.Errorf("the target holds %q, %v; want its old bytes", got, err)
				}
			default:
				absent(t, target)
			}
			noTemporary(t, filepath.Dir(p))
		})
	}
}

// errRemove is the failure a test's discard returns.
var errRemove = errors.New("the temporary could not be removed")

// TestSETUP1_CF8_ARemovalThatFailsAfterTheLinkNamesTheLeftover (pb40 L1): the
// publish stands, the error is a *LeftError naming the temporary, and the
// directory lists the file and that one temporary, nothing else.
func TestSETUP1_CF8_ARemovalThatFailsAfterTheLinkNamesTheLeftover(t *testing.T) {
	was := discard
	discard = func(*os.Root, string) error { return errRemove }
	t.Cleanup(func() { discard = was })
	dir := t.TempDir()
	err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew)
	var left *LeftError
	if !errors.As(err, &left) || !errors.Is(err, errRemove) {
		t.Fatalf("CreateFile = %v; want a *LeftError wrapping the removal's error", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, setupName))
	if err != nil || string(got) != string(newBytes) {
		t.Errorf("name holds %q, %v; want the whole file", got, err)
	}
	if !strings.HasPrefix(filepath.ToSlash(left.Temp), ".rashomon/.graph.json.tmp-") {
		t.Errorf("Temp = %q; want the temporary beside name, under the root", left.Temp)
	}
	if got, want := entries(t, filepath.Join(dir, ".rashomon")), []string{filepath.Base(left.Temp), "graph.json"}; !slices.Equal(got, want) {
		t.Errorf(".rashomon holds %v; want %v", got, want)
	}
}

// TestSETUP1_CF9_AParentThatIsALinkRefusesTheCreate (pb40 L2): .rashomon as a
// link to another directory of the root refuses the create; the link reads
// as it did and its target directory gets no file through it.
func TestSETUP1_CF9_AParentThatIsALinkRefusesTheCreate(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, "other", filepath.Join(dir, ".rashomon"))
	if err := CreateFile(openRoot(t, dir), setupName, 0o644, roomy, writeNew); err == nil {
		t.Fatal("CreateFile through a parent link = nil; want a refusal")
	}
	if got, err := os.Readlink(filepath.Join(dir, ".rashomon")); err != nil || got != "other" {
		t.Errorf("the link reads %q, %v; want other", got, err)
	}
	if got := entries(t, filepath.Join(dir, "other")); len(got) != 0 {
		t.Errorf("other holds %v; want nothing written through the link", got)
	}
}
