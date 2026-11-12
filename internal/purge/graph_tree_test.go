package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestPurge_OwnsGraphTree (W-M2.11 1d): the graph directory's session log
// and the hook path's evidence file are rashomon's, so a store holding only
// them is fully owned and the root goes; a person's file, a dotfile, a
// subdirectory and a symlink named like an evidence file are foreign.
func TestPurge_OwnsGraphTree(t *testing.T) {
	evidence := graph.EvidenceFileName("0b6a9c52-1111-4222-8333-444455556666", "7c1d2e3f-aaaa-4bbb-8ccc-ddddeeeeffff", time.Now())
	log := filepath.Base(graph.SessionLogPath("", "github.com/x/y"))
	if !graph.IsSessionLogName(log) || !graph.IsEvidenceFileName(evidence) {
		t.Fatalf("the specimen names are not the writers' own: %q %q", log, evidence)
	}
	build := func(t *testing.T, put func(dir string)) Plan {
		t.Helper()
		root := t.TempDir()
		for _, a := range []string{InstallMeta, InstallKey} {
			if err := os.WriteFile(filepath.Join(root, a), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		dir := filepath.Join(root, graph.StoreDir)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		put(dir)
		p, err := Build(root, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	file := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			panic(err)
		}
	}

	own := build(t, func(dir string) { file(dir, log); file(dir, evidence) })
	if !own.RemoveRoot || len(own.Foreign) != 0 {
		t.Errorf("the writers' own files are not fully owned: RemoveRoot %v, Foreign %q", own.RemoveRoot, own.Foreign)
	}
	for _, name := range []string{log, evidence} {
		if !slices.Contains(own.Files, filepath.Join(own.Root, graph.StoreDir, name)) {
			t.Errorf("%s is not planned for removal: %q", name, own.Files)
		}
	}
	if !slices.Contains(own.Files, filepath.Join(own.Root, graph.StoreDir)) {
		t.Errorf("the emptied graph directory is not planned for removal: %q", own.Files)
	}

	for name, put := range map[string]func(dir string){
		"a user's notes.txt":              func(dir string) { file(dir, "notes.txt") },
		"a dotfile":                       func(dir string) { file(dir, ".x-y.txt") },
		"a subdirectory":                  func(dir string) { _ = os.MkdirAll(filepath.Join(dir, "sub"), 0o700) },
		"a directory named like a log":    func(dir string) { _ = os.MkdirAll(filepath.Join(dir, log), 0o700) },
		"a symlink named like evidence":   func(dir string) { _ = os.Symlink(filepath.Join(dir, "..", InstallMeta), filepath.Join(dir, evidence)) },
		"a file in an evidence-named dir": func(dir string) { d := filepath.Join(dir, evidence); _ = os.MkdirAll(d, 0o700); file(d, log) },
	} {
		p := build(t, put)
		if p.RemoveRoot || len(p.Foreign) == 0 {
			t.Errorf("%s: RemoveRoot %v, Foreign %q, want the root kept", name, p.RemoveRoot, p.Foreign)
		}
	}
}
