package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestPurge_RemovesOverlayRunFiles: the two files the overlay writers leave
// in a run directory (the byte counter and the journal-full marker) are
// rashomon's, so the plan lists them before their run directory and nothing
// is foreign: a run file purge did not know would keep the store root from
// being removed.
func TestPurge_RemovesOverlayRunFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rashomon")
	var ours []string
	for _, f := range []string{InstallMeta, InstallKey} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{store.FileRecords, store.FileOverlayBytes, store.FileOverlayFull} {
		ours = append(ours, "runs/s1/"+f)
	}
	for _, f := range ours {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	at := func(f string) int { return slices.Index(p.Files, filepath.Join(root, filepath.FromSlash(f))) }
	dir := at("runs/s1")
	for _, f := range ours {
		if i := at(f); i < 0 || dir < 0 || i > dir {
			t.Errorf("the plan does not list %s before its run directory (file %d, directory %d)", f, i, dir)
		}
	}
	if len(p.Foreign) != 0 || !p.RemoveRoot {
		t.Errorf("foreign %v, RemoveRoot %v: want nothing foreign and the root removed", p.Foreign, p.RemoveRoot)
	}
}
