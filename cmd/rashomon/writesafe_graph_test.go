package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSafeOutBytes is what a file outside the repository holds; no graph verb
// run in the repository may change it.
var writeSafeOutBytes = []byte("bytes outside the repository, which no graph verb may change\n")

// writeSafeLink makes link a symlink to target, or skips the test: Windows
// creates one only with the privilege or developer mode.
func writeSafeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
}

// writeSafeLoud fails unless err is one line naming path.
func writeSafeLoud(t *testing.T, what string, err error, path string) {
	t.Helper()
	switch {
	case err == nil:
		t.Errorf("%s followed a link out of the repository and returned no error", what)
	case !strings.Contains(err.Error(), path):
		t.Errorf("%s = %v; want an error naming %s", what, err, path)
	case strings.Contains(err.Error(), "\n"):
		t.Errorf("%s = %q; want one line, which main prints after \"rashomon:\"", what, err)
	}
}

// writeSafeKept fails unless dir holds exactly names and file still reads
// writeSafeOutBytes.
func writeSafeKept(t *testing.T, dir, file string, names ...string) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(dir, file)); err != nil || !bytes.Equal(got, writeSafeOutBytes) {
		t.Errorf("%s outside the repository now reads %q (err %v); want it unchanged", file, got, err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
	}
	if strings.Join(got, " ") != strings.Join(names, " ") {
		t.Errorf("the directory outside the repository holds %v; want %v", got, names)
	}
}

// TestWS1_I7_GraphIndexFailsLoudOnALinkedIndexDirOutside: graph index
// in a repository whose .rashomon is a link out of it fails with one line
// naming the link, and the index at the link's target is byte-equal, with
// no temp beside it. Break: save the index through the link.
func TestWS1_I7_GraphIndexFailsLoudOnALinkedIndexDirOutside(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	moved := filepath.Join(t.TempDir(), "rashomon")
	if err := os.Rename(filepath.Join(dir, ".rashomon"), moved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moved, "graph.jsonl"), writeSafeOutBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSafeLink(t, moved, filepath.Join(dir, ".rashomon"))
	writeSafeLoud(t, "graph index", graphIndex([]string{dir}, io.Discard), filepath.Join(dir, ".rashomon"))
	writeSafeKept(t, moved, "graph.jsonl", "graph.json", "graph.jsonl")
}

// TestWS1_I7k_GraphKnowledgeImportFailsLoudOnALinkedKnowledgeDirOutside:
// graph knowledge import in a repository whose knowledge directory is a
// link out of it fails with one line naming the link, and the row file at
// the link's target is byte-equal. Break: write the rows through the link.
func TestWS1_I7k_GraphKnowledgeImportFailsLoudOnALinkedKnowledgeDirOutside(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	if err := graphIndex([]string{dir}, io.Discard); err != nil {
		t.Fatalf("graph index before the link: %v", err)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "ledger.json"), writeSafeOutBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	kn := filepath.Join(dir, ".rashomon", "knowledge")
	writeSafeLink(t, out, kn)
	src := filepath.Join(t.TempDir(), "LEDGER.txt")
	if err := os.WriteFile(src, []byte("id | when | who | what | layer | evidence | catcher | today | gap\n"+
		"B1 | 11-01 | me | changed the session ttl without jobs/purge_sessions.py | purge / DETECT | run 3 | me | no | the nightly purge must change with session lifetime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(dir, "services", "auth"))
	writeSafeLoud(t, "graph knowledge import", cmdGraph([]string{"knowledge", "import", src}, io.Discard), kn)
	writeSafeKept(t, out, "ledger.json", "ledger.json")
}
