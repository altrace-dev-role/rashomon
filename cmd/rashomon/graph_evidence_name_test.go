package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestGraphEvidence_NameStaysInGraphDir (W-M2.11 1d): the evidence file's
// name is built from the hook's session and prompt ids, which arrive from
// outside; an id holding "/../.." must not move the file out of
// <store>/graph. Each row would land its escape inside the test's own
// TempDir, and every file written is a direct child of <store>/graph named
// by the one rule purge reads back. The store's record_scope marker says
// v5, the scope the file waits on (writeGraphEvidence), and is the one other
// file the test writes.
func TestGraphEvidence_NameStaysInGraphDir(t *testing.T) {
	for _, tc := range []struct{ name, session, prompt string }{
		{"plain ids", "0b6a9c52-1111-4222-8333-444455556666", "7c1d2e3f-aaaa-4bbb-8ccc-ddddeeeeffff"},
		{"hostile prompt, rooted", "s1", "/../../escaped"},
		{"hostile prompt, relative", "s1", "../../escaped"},
		{"hostile prompt, separator", "s1", "a/b"},
		{"hostile session, rooted", "/../../escaped", "p1"},
		{"hostile session, relative", "../../escaped", "p1"},
		{"hostile session, separator", "a/b", "p1"},
		{"no prompt id", "s1", ""},
		{"dotfile session", ".hidden", "p1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			graphProject(t, dir)
			base := t.TempDir()
			storeRoot := filepath.Join(base, "a", "b", "store")
			if err := os.MkdirAll(storeRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := store.WriteRecordScopeMarker(storeRoot, store.RecordScopeV5); err != nil {
				t.Fatal(err)
			}
			scopeMarker := filepath.Join(storeRoot, store.FileRecordScope)
			if _, ok := graphBrief(storeRoot, filepath.Join(dir, "services", "auth"), tc.session, tc.prompt, []byte("purging the sessions")); !ok {
				t.Fatal("no brief")
			}
			var files []string
			err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && p != scopeMarker {
					files = append(files, p)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 {
				t.Fatalf("want one file written under the TempDir beside the marker, got %v", files)
			}
			rel, _ := filepath.Rel(filepath.Join(storeRoot, graph.StoreDir), files[0])
			if rel != filepath.Base(files[0]) || !graph.IsEvidenceFileName(rel) {
				t.Errorf("%s is not a direct child of <store>/graph named by the evidence rule", files[0])
			}
			if _, err := os.Stat(filepath.Join(storeRoot, graph.StoreDir, rel)); err != nil {
				t.Error(err)
			}
		})
	}
}
