package purge

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestPurge_OwnsClearTree (x35.5, x41.3): the /clear rendezvous's files are
// rashomon's: clear/<hex of the SHA-256 of a cwd's path>/ holding end-<id>,
// start-<id>, link-<id> and a link's temporary .link-<id>.tmp-<n>. A store
// holding only them is fully owned: each file, each folder and clear/ are
// planned for removal and the root goes. Any other name is left, and so is
// every folder above it, while a folder holding only rashomon's names still
// goes: a person's file in a digest folder, a folder whose name is not a
// lowercase digest, a file at the top of clear/ and a symlink named like an
// arm. Purge has no session scope. The names are written here as literals,
// not taken from the store's helpers. Break: leave clear out of the walk,
// take a foreign name or folder, follow a link, or keep a clean folder
// because another holds a foreign name.
func TestPurge_OwnsClearTree(t *testing.T) {
	d1, d2 := sha256.Sum256([]byte("/w/p")), sha256.Sum256([]byte("/w/q"))
	f1, f2 := hex.EncodeToString(d1[:]), hex.EncodeToString(d2[:])
	const (
		oldID  = "0b6a9c52-1111-4222-8333-444455556666"
		newID  = "7c1d2e3f-aaaa-4bbb-8ccc-ddddeeeeffff"
		loneID = "5e4d3c2b-9999-4888-8777-666655554444"
	)
	file := func(dir, name string) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			panic(err)
		}
	}
	owned := func(c string) {
		file(filepath.Join(c, f1), "end-"+oldID)
		file(filepath.Join(c, f1), "start-"+newID)
		file(filepath.Join(c, f1), "link-"+newID)
		file(filepath.Join(c, f1), ".link-"+newID+".tmp-123")
		file(filepath.Join(c, f2), "end-"+loneID)
	}
	build := func(t *testing.T, put func(c string)) Plan {
		t.Helper()
		root := t.TempDir()
		for _, a := range []string{InstallMeta, InstallKey} {
			file(root, a)
		}
		c := filepath.Join(root, "clear")
		owned(c)
		put(c)
		p, err := Build(root, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	own := build(t, func(string) {})
	if !own.RemoveRoot || len(own.Foreign) != 0 {
		t.Errorf("the rendezvous files are not fully owned: RemoveRoot %v, %d foreign", own.RemoveRoot, len(own.Foreign))
	}
	for _, rel := range []string{
		f1 + "/end-" + oldID, f1 + "/start-" + newID, f1 + "/link-" + newID, f1 + "/.link-" + newID + ".tmp-123",
		f2 + "/end-" + loneID, f1, f2, "",
	} {
		if !slices.Contains(own.Files, filepath.Join(own.Root, "clear", filepath.FromSlash(rel))) {
			t.Errorf("clear/%s is not planned for removal", rel)
		}
	}

	for name, put := range map[string]func(c string){
		"a user's notes.txt in a digest folder": func(c string) { file(filepath.Join(c, f1), "notes.txt") },
		"a folder whose name is not a digest":   func(c string) { file(filepath.Join(c, "proj"), "end-"+oldID) },
		"an uppercase digest folder":            func(c string) { file(filepath.Join(c, strings.ToUpper(f1)), "end-"+oldID) },
		"a file at the top of clear":            func(c string) { file(c, "end-"+oldID) },
		"a symlink named like an arm": func(c string) {
			_ = os.Symlink(filepath.Join(c, f2, "end-"+loneID), filepath.Join(c, f1, "start-"+loneID))
		},
	} {
		p := build(t, put)
		if p.RemoveRoot || len(p.Foreign) == 0 {
			t.Errorf("%s: RemoveRoot %v, %d foreign; want the root kept", name, p.RemoveRoot, len(p.Foreign))
		}
		if slices.Contains(p.Files, filepath.Join(p.Root, "clear")) {
			t.Errorf("%s: clear/ is planned for removal while it holds a foreign name", name)
		}
		if !slices.Contains(p.Files, filepath.Join(p.Root, "clear", f2)) {
			t.Errorf("%s: the folder holding only rashomon's names is not planned for removal", name)
		}
	}
}
