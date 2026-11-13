package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// frontierStore is a store at a fresh root with the run directory of each id
// made, as a session's first record makes it.
func frontierStore(t *testing.T, ids ...string) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := os.MkdirAll(s.RunDir(id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// treeOf is every path under root, relative and slash-separated, sorted.
func treeOf(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		paths = append(paths, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(paths)
	return paths
}

// sizedFile makes path a file of size bytes, every one zero.
func sizedFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestFrontierFile_SaveReplacesAndLoadReturnsIt (W-M4.2 I/O slice, ruling
// 15:29 (3)): a save lands at runs/<session>/frontier.json, a regular file at
// 0600; a shorter second save replaces the first whole; a load returns the
// last save's bytes and true; and the run directory holds that one file, no
// temporary left beside it.
func TestFrontierFile_SaveReplacesAndLoadReturnsIt(t *testing.T) {
	s := frontierStore(t, "sess-1")
	for _, data := range [][]byte{[]byte(`{"schema":1,"through":{"seq":9}}` + "\n"), []byte(`{"schema":1}` + "\n")} {
		if err := s.SaveFrontier("sess-1", data); err != nil {
			t.Fatalf("SaveFrontier: %v", err)
		}
		if got, ok, err := s.LoadFrontier("sess-1"); err != nil || !ok || !bytes.Equal(got, data) {
			t.Fatalf("LoadFrontier = (%q, %v, %v); want (%q, true, nil)", got, ok, err, data)
		}
	}
	path := filepath.Join(s.root, DirRuns, "sess-1", FileFrontier)
	if fi, err := os.Lstat(path); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Fatalf("%s: %v, %v; want a regular file, mode 0600", path, fi, err)
	}
	if got := treeOf(t, s.RunDir("sess-1")); !slices.Equal(got, []string{".", FileFrontier}) {
		t.Errorf("the run directory holds %q; want only %s, no temporary", got, FileFrontier)
	}
}

// TestFrontierFile_AbsentIsNoError: a session with no run directory, and one
// whose run directory holds no frontier, both load as absent: nil, false and
// no error, so the caller folds from the record.
func TestFrontierFile_AbsentIsNoError(t *testing.T) {
	s := frontierStore(t, "sess-empty")
	for _, id := range []string{"sess-none", "sess-empty"} {
		if got, ok, err := s.LoadFrontier(id); got != nil || ok || err != nil {
			t.Errorf("LoadFrontier(%s) = (%q, %v, %v); want (nil, false, nil)", id, got, ok, err)
		}
	}
}

// TestFrontierFile_MapsAnUnsafeID (ruling 15:29 (3)): an id unsafe as a path
// element is mapped as RunDir maps it, to its digest, and never refused: the
// save lands at runs/x-<digest>/frontier.json, nothing is written where the
// raw id points (a directory made there to catch it), and a load by the same
// id reads the bytes back.
func TestFrontierFile_MapsAnUnsafeID(t *testing.T) {
	const id = "../escape"
	s := frontierStore(t, id)
	raw := filepath.Join(s.root, "escape")
	if err := os.Mkdir(raw, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schema":1}` + "\n")
	if err := s.SaveFrontier(id, data); err != nil {
		t.Fatalf("SaveFrontier(%q): %v", id, err)
	}
	mapped := filepath.Join(s.root, DirRuns, digestID(id), FileFrontier)
	if b, err := os.ReadFile(mapped); err != nil || !bytes.Equal(b, data) {
		t.Errorf("%s = (%q, %v); want the saved bytes", mapped, b, err)
	}
	if got := treeOf(t, raw); !slices.Equal(got, []string{"."}) {
		t.Errorf("the raw id was joined: %s holds %q", raw, got)
	}
	if got, ok, err := s.LoadFrontier(id); err != nil || !ok || !bytes.Equal(got, data) {
		t.Errorf("LoadFrontier(%q) = (%q, %v, %v); want the saved bytes", id, got, ok, err)
	}
}

// TestFrontierFile_MissingRunDirWritesNothing (ruling 15:29 (3)): the save
// makes no run directory; into a missing one it returns the error, matching
// fs.ErrNotExist, and the store's tree is as it was.
func TestFrontierFile_MissingRunDirWritesNothing(t *testing.T) {
	s := frontierStore(t)
	before := treeOf(t, s.root)
	if err := s.SaveFrontier("sess-missing", []byte("{}\n")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("SaveFrontier into a missing run directory: %v; want an error matching fs.ErrNotExist", err)
	}
	if after := treeOf(t, s.root); !slices.Equal(before, after) {
		t.Errorf("the store's tree changed:\nbefore %q\nafter  %q", before, after)
	}
}

// TestFrontierFile_UnreadableIsAnError: a path that is not a regular file (a
// directory here; regfile's own tests hold the named pipe) and a file past
// MaxFrontierBytes each load as unreadable: nil, true and an error, and the
// caller folds from the record. A file of exactly MaxFrontierBytes reads
// whole.
func TestFrontierFile_UnreadableIsAnError(t *testing.T) {
	s := frontierStore(t, "sess-dir", "sess-cap", "sess-over")
	if err := os.Mkdir(filepath.Join(s.RunDir("sess-dir"), FileFrontier), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.LoadFrontier("sess-dir"); got != nil || !ok || !errors.Is(err, regfile.ErrNotRegular) {
		t.Errorf("a directory at the path: (%q, %v, %v); want (nil, true, ErrNotRegular)", got, ok, err)
	}
	sizedFile(t, filepath.Join(s.RunDir("sess-cap"), FileFrontier), MaxFrontierBytes)
	sizedFile(t, filepath.Join(s.RunDir("sess-over"), FileFrontier), MaxFrontierBytes+1)
	if got, ok, err := s.LoadFrontier("sess-cap"); err != nil || !ok || len(got) != MaxFrontierBytes {
		t.Errorf("a file of MaxFrontierBytes: (%d bytes, %v, %v); want it read whole", len(got), ok, err)
	}
	if got, ok, err := s.LoadFrontier("sess-over"); got != nil || !ok || !errors.Is(err, errFrontierOverCap) {
		t.Errorf("a file past MaxFrontierBytes: (%d bytes, %v, %v); want (nil, true, errFrontierOverCap)", len(got), ok, err)
	}
}

// TestFrontierFile_SaveOverCapKeepsTheLast: data past MaxFrontierBytes is
// refused with errFrontierOverCap before anything is written, so the tree is
// as it was and the last frontier still loads; data of exactly
// MaxFrontierBytes saves, and loads whole.
func TestFrontierFile_SaveOverCapKeepsTheLast(t *testing.T) {
	s := frontierStore(t, "sess-1")
	last := []byte(`{"schema":1}` + "\n")
	if err := s.SaveFrontier("sess-1", last); err != nil {
		t.Fatalf("SaveFrontier: %v", err)
	}
	before := treeOf(t, s.root)
	if err := s.SaveFrontier("sess-1", make([]byte, MaxFrontierBytes+1)); !errors.Is(err, errFrontierOverCap) {
		t.Errorf("a save past MaxFrontierBytes: %v; want errFrontierOverCap", err)
	}
	if after := treeOf(t, s.root); !slices.Equal(before, after) {
		t.Errorf("the refused save changed the tree:\nbefore %q\nafter  %q", before, after)
	}
	if got, ok, err := s.LoadFrontier("sess-1"); err != nil || !ok || !bytes.Equal(got, last) {
		t.Errorf("after the refused save: (%d bytes, %v, %v); want the last frontier", len(got), ok, err)
	}
	if err := s.SaveFrontier("sess-1", bytes.Repeat([]byte{'x'}, MaxFrontierBytes)); err != nil {
		t.Fatalf("a save of exactly MaxFrontierBytes: %v", err)
	}
	if got, ok, err := s.LoadFrontier("sess-1"); err != nil || !ok || len(got) != MaxFrontierBytes {
		t.Errorf("a frontier of MaxFrontierBytes: (%d bytes, %v, %v); want it whole", len(got), ok, err)
	}
}

// TestFrontierFile_AZeroStoreRefuses: as each writer DOCS9 names, a Store not
// from Open refuses both calls with errUnopenedStore. A frontier of some other
// tree stands where the zero Store's relative path points, under the working
// directory: the load does not return it, the save does not replace it, and
// nothing else is written there.
func TestFrontierFile_AZeroStoreRefuses(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := os.MkdirAll(filepath.Join(DirRuns, "s"), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte(`{"schema":1,"foreign":true}` + "\n")
	planted := filepath.Join(DirRuns, "s", FileFrontier)
	if err := os.WriteFile(planted, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	var st Store
	if got, ok, err := st.LoadFrontier("s"); got != nil || ok || !errors.Is(err, errUnopenedStore) {
		t.Errorf("LoadFrontier on a zero Store: (%q, %v, %v); want (nil, false, errUnopenedStore)", got, ok, err)
	}
	if err := st.SaveFrontier("s", []byte("{}\n")); !errors.Is(err, errUnopenedStore) {
		t.Errorf("SaveFrontier on a zero Store: %v; want errUnopenedStore", err)
	}
	if b, err := os.ReadFile(planted); err != nil || !bytes.Equal(b, foreign) {
		t.Errorf("the frontier under the working directory is now (%q, %v); want it untouched", b, err)
	}
	if got := treeOf(t, cwd); !slices.Equal(got, []string{".", DirRuns, DirRuns + "/s", DirRuns + "/s/" + FileFrontier}) {
		t.Errorf("a zero Store wrote under the working directory: %q", got)
	}
}
