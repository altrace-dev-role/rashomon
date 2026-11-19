package graph

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

// writeSafeOutsideBytes is what a file outside every root a test saves under holds;
// no save may change it.
var writeSafeOutsideBytes = []byte("bytes outside the root, which no save under it may change\n")

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
		t.Errorf("%s returned no error through a link it must refuse", what)
	case !strings.Contains(err.Error(), path):
		t.Errorf("%s = %v; want an error naming %s", what, err, path)
	case strings.Contains(err.Error(), "\n"):
		t.Errorf("%s = %q; want one line", what, err)
	}
}

// TestWS1_I1_SaveFileRefusesALinkedIndexDirLeavingTheRoot: a .rashomon that is
// a link out of the root is refused, with an error naming it, and the file
// at the link's target is byte-equal after. Break: save through the link
// (MkdirAll, CreateTemp and Rename by path all follow it).
func TestWS1_I1_SaveFileRefusesALinkedIndexDirLeavingTheRoot(t *testing.T) {
	root := t.TempDir()
	out := writeSafeOutsideDir(t, "graph.jsonl")
	writeSafeLink(t, out, filepath.Join(root, ".rashomon"))
	err := SaveFile(root, NewStore(), Header{Repo: "r", Rev: HeaderRev{Commit: "C1"}})
	writeSafeRefused(t, "SaveFile", err, filepath.Join(root, ".rashomon"))
	writeSafeOutsideUnchanged(t, out, "graph.jsonl")
}

// TestWS1_I5_SaveFileReplacesALinkAtTheIndexFile: a graph.jsonl that
// is a link out of the root is replaced by the saved index, and its target
// is byte-equal: the rename takes the name, not what it points at. Break:
// write the index by opening graph.jsonl.
func TestWS1_I5_SaveFileReplacesALinkAtTheIndexFile(t *testing.T) {
	root := t.TempDir()
	out := writeSafeOutsideDir(t, "graph.jsonl")
	if err := os.MkdirAll(filepath.Join(root, ".rashomon"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, filepath.FromSlash(GraphFile))
	writeSafeLink(t, filepath.Join(out, "graph.jsonl"), p)
	if err := SaveFile(root, NewStore(), Header{Repo: "r", Rev: HeaderRev{Commit: "C1"}}); err != nil {
		t.Fatalf("SaveFile over a link at %s: %v", p, err)
	}
	writeSafeOutsideUnchanged(t, out, "graph.jsonl")
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("%s after the save: %v (err %v); want a regular file", p, fi, err)
	}
}

// TestWS1_I10_SaveFileFollowsARelativeLinkInsideTheRoot: a .rashomon that is a
// relative link to a directory inside the root is followed, and the index
// reads back through it. This is the limit of the refusal: a link that
// stays inside the root is the person's arrangement. Break: refuse every
// link at .rashomon.
func TestWS1_I10_SaveFileFollowsARelativeLinkInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSafeLink(t, "elsewhere", filepath.Join(root, ".rashomon"))
	if err := SaveFile(root, NewStore(), Header{Repo: "r", Rev: HeaderRev{Commit: "C1"}}); err != nil {
		t.Fatalf("SaveFile through a link inside the root: %v", err)
	}
	if fi, err := os.Lstat(filepath.Join(root, "elsewhere", "graph.jsonl")); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("elsewhere/graph.jsonl: %v (err %v); want the saved index", fi, err)
	}
	if _, h, err := LoadFile(root); err != nil || h.Repo != "r" {
		t.Errorf("LoadFile through the link: repo %q, err %v", h.Repo, err)
	}
}

// TestWS1_I10b_SaveFileRefusesAnAbsoluteLinkEvenInsideTheRoot: a .rashomon that
// is an absolute link is refused even when it names a directory inside the
// root, because a root-scoped open cannot tell an absolute path inside from
// one outside. The stated limit: only a relative link that stays inside is
// followed. Break: follow absolute links.
func TestWS1_I10b_SaveFileRefusesAnAbsoluteLinkEvenInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "elsewhere")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSafeLink(t, inside, filepath.Join(root, ".rashomon"))
	err := SaveFile(root, NewStore(), Header{Repo: "r", Rev: HeaderRev{Commit: "C1"}})
	writeSafeRefused(t, "SaveFile", err, filepath.Join(root, ".rashomon"))
	if ents, _ := os.ReadDir(inside); len(ents) != 0 {
		t.Errorf("elsewhere holds %d entries after a refused save; want none", len(ents))
	}
}
