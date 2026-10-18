package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTRU1_WorkTreeTopFindsTheTreeARootWouldBeIn: the top of the work tree
// is found for a root that does not exist yet, through a symlinked parent,
// and for a .git that is a file (a linked work tree); outside one it is "".
func TestTRU1_WorkTreeTopFindsTheTreeARootWouldBeIn(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if top, err := WorkTreeTop(base); err != nil || top != "" {
		t.Skipf("premise: the temporary directory is itself inside a work tree (%q, %v)", top, err)
	}
	project := filepath.Join(base, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(project, "state", "rashomon")
	if top, err := WorkTreeTop(root); err != nil || top != project {
		t.Errorf("WorkTreeTop(%s) = %q, %v; want %s", root, top, err, project)
	}
	if top, err := WorkTreeTop(filepath.Join(base, "elsewhere", "rashomon")); err != nil || top != "" {
		t.Errorf("WorkTreeTop outside a work tree = %q, %v; want \"\"", top, err)
	}

	linked := filepath.Join(base, "linked")
	if err := os.MkdirAll(linked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if top, err := WorkTreeTop(filepath.Join(linked, "a", "b")); err != nil || top != linked {
		t.Errorf("WorkTreeTop under a .git file = %q, %v; want %s", top, err, linked)
	}

	if runtime.GOOS == "windows" {
		return
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	if top, err := WorkTreeTop(filepath.Join(alias, "state")); err != nil || top != project {
		t.Errorf("WorkTreeTop through a symlink = %q, %v; want the resolved %s", top, err, project)
	}
}

// TestTRU1_SetRootInWorkTreeKeepsTheRestOfInstallJSON: the confirmed top is
// recorded under root_in_work_tree and every other key is left as read; a
// root with no install.json is refused, not created.
func TestTRU1_SetRootInWorkTreeKeepsTheRestOfInstallJSON(t *testing.T) {
	root := t.TempDir()
	if err := SetRootInWorkTree(root, "/w"); !errors.Is(err, ErrNoStore) {
		t.Fatalf("SetRootInWorkTree on an empty root = %v, want ErrNoStore", err)
	}
	if _, err := os.Stat(filepath.Join(root, FileInstallMeta)); !os.IsNotExist(err) {
		t.Fatal("SetRootInWorkTree created install.json")
	}
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, FileInstallMeta)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var before map[string]any
	if err := json.Unmarshal(b, &before); err != nil {
		t.Fatal(err)
	}
	before["created_dirs"] = []any{"/x"}
	seeded, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, seeded, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetRootInWorkTree(root, "/home/u/project"); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	if err := json.Unmarshal(b, &after); err != nil {
		t.Fatal(err)
	}
	if after[KeyRootInWorkTree] != "/home/u/project" {
		t.Errorf("root_in_work_tree = %v, want /home/u/project", after[KeyRootInWorkTree])
	}
	for k, v := range before {
		got, _ := json.Marshal(after[k])
		want, _ := json.Marshal(v)
		if string(got) != string(want) {
			t.Errorf("%s = %s after the write, want %s", k, got, want)
		}
	}
	if id, err := ReadInstallID(root); err != nil || id != st.InstallID() {
		t.Errorf("ReadInstallID after the write = %q, %v; want %s", id, err, st.InstallID())
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("install.json mode after the write = %o, want 600", info.Mode().Perm())
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if ok, _ := filepath.Match("."+FileInstallMeta+".tmp-*", e.Name()); ok {
			t.Errorf("the replacement %s was left behind", e.Name())
		}
	}
}
