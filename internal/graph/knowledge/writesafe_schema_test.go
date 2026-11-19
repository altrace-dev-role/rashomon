package knowledge

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSafeLink makes link a symlink to target, or skips the test: Windows
// creates one only with the privilege or developer mode, and a row that
// cannot lay its link down holds nothing.
func writeSafeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
}

// writeSafeOutsideBytes is what a file outside every root a test writes under holds;
// no write may change it.
var writeSafeOutsideBytes = []byte("bytes outside the root, which no write under it may change\n")

// writeSafeOutsideDir is a fresh directory, outside the test's root, holding name
// with writeSafeOutsideBytes.
func writeSafeOutsideDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), writeSafeOutsideBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeSafeOutsideUnchanged fails unless dir still holds name, byte-equal, and
// nothing else: a temp left beside it is a write outside the root too.
func writeSafeOutsideUnchanged(t *testing.T, dir, name string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || !bytes.Equal(got, writeSafeOutsideBytes) {
		t.Errorf("%s outside the root now reads %q (err %v); want it unchanged", name, got, err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Errorf("the directory outside the root holds %v; want only %s", names, name)
	}
}

// writeSafeRefused fails unless err is a one-line error naming path.
func writeSafeRefused(t *testing.T, what string, err error, path string) {
	t.Helper()
	switch {
	case err == nil:
		t.Errorf("%s followed a link out of the root and returned no error", what)
	case !strings.Contains(err.Error(), path):
		t.Errorf("%s = %v; want an error naming %s", what, err, path)
	case strings.Contains(err.Error(), "\n"):
		t.Errorf("%s = %q; want one line", what, err)
	}
}

// TestWS1_I1k_WriteFileRefusesALinkedRashomonDirLeavingTheRoot: a .rashomon that
// is a link out of the root is refused, with an error naming it, and the
// row file at the link's target is byte-equal after. Break: write through
// the link (MkdirAll and WriteFile by path follow it).
func TestWS1_I1k_WriteFileRefusesALinkedRashomonDirLeavingTheRoot(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	kn := filepath.Join(out, "knowledge")
	if err := os.Mkdir(kn, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kn, "x.json"), writeSafeOutsideBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSafeLink(t, out, filepath.Join(root, ".rashomon"))
	_, err := WriteFile(root, "x", nil)
	writeSafeRefused(t, "WriteFile", err, filepath.Join(root, ".rashomon"))
	writeSafeOutsideUnchanged(t, kn, "x.json")
}

// TestWS1_I3_WriteFileRefusesALinkedKnowledgeDirLeavingTheRoot: the same for a
// knowledge directory that is a link out of the root. Break: write through
// the link.
func TestWS1_I3_WriteFileRefusesALinkedKnowledgeDirLeavingTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".rashomon"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := writeSafeOutsideDir(t, "x.json")
	writeSafeLink(t, out, filepath.Join(root, filepath.FromSlash(Dir)))
	_, err := WriteFile(root, "x", nil)
	writeSafeRefused(t, "WriteFile", err, filepath.Join(root, filepath.FromSlash(Dir)))
	writeSafeOutsideUnchanged(t, out, "x.json")
}

// TestWS1_I4_WriteFileReplacesALinkAtTheRowFile: a name.json that is
// a link out of the root is replaced by the written rows, as the index's
// graph.jsonl is, and its target is byte-equal. Break: write the rows by
// opening name.json (os.WriteFile truncates the link's target).
func TestWS1_I4_WriteFileReplacesALinkAtTheRowFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(Dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	out := writeSafeOutsideDir(t, "x.json")
	p := filepath.Join(root, filepath.FromSlash(Dir), "x.json")
	writeSafeLink(t, filepath.Join(out, "x.json"), p)
	got, err := WriteFile(root, "x", nil)
	if err != nil || got != p {
		t.Fatalf("WriteFile over a link at %s = %q, %v; want %s, nil", p, got, err, p)
	}
	writeSafeOutsideUnchanged(t, out, "x.json")
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("%s after the write: %v (err %v); want a regular file", p, fi, err)
	}
}

// TestWS1_I10k_WriteFileFollowsARelativeLinkInsideTheRoot: a knowledge directory
// that is a relative link to a directory inside the root is followed. The
// limit of the refusal. Break: refuse every link at the knowledge directory.
func TestWS1_I10k_WriteFileFollowsARelativeLinkInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{".rashomon", "kn"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeSafeLink(t, filepath.Join("..", "kn"), filepath.Join(root, filepath.FromSlash(Dir)))
	if _, err := WriteFile(root, "x", nil); err != nil {
		t.Fatalf("WriteFile through a link inside the root: %v", err)
	}
	if fi, err := os.Lstat(filepath.Join(root, "kn", "x.json")); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("kn/x.json: %v (err %v); want the written rows", fi, err)
	}
}
