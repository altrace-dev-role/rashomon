package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestTRU44_InstallRecordUpdateKeepsTheIDAndUnknownKeys: watch's update of
// install.json replaces what it owns and nothing else -- a key a later
// release writes survives, the install id cannot be edited -- and never
// creates the file.
func TestTRU44_InstallRecordUpdateKeepsTheIDAndUnknownKeys(t *testing.T) {
	root := t.TempDir()
	if err := UpdateInstallRecord(root, func(*InstallRecord) {}); !errors.Is(err, ErrNoStore) {
		t.Fatalf("update with no install.json = %v, want ErrNoStore", err)
	}
	if _, err := os.Stat(filepath.Join(root, FileInstallMeta)); err == nil {
		t.Fatal("an update created install.json")
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
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	doc["later_release"] = "kept"
	b, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}

	existed := false
	if err := UpdateInstallRecord(root, func(r *InstallRecord) {
		r.InstallID = "ffffffffffffffffffffffffffffffff"
		r.SettingsExisted = &existed
		r.PreSHA256, r.PostSHA256 = "aa", "bb"
		r.CreatedDirs = []string{root}
	}); err != nil {
		t.Fatal(err)
	}
	rec, err := ReadInstallRecord(root)
	if err != nil {
		t.Fatal(err)
	}
	if rec.InstallID != st.InstallID() {
		t.Errorf("the update changed the install id to %q", rec.InstallID)
	}
	if rec.SettingsExisted == nil || *rec.SettingsExisted || rec.PreSHA256 != "aa" || rec.PostSHA256 != "bb" || len(rec.CreatedDirs) != 1 {
		t.Errorf("the update did not land: %+v", rec)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc = nil
	if err := json.Unmarshal(b, &doc); err != nil || doc["later_release"] != "kept" {
		t.Errorf("a key the record does not own was dropped: %s", b)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != fileMode {
		t.Errorf("install.json mode after the update: %v %v", info.Mode(), err)
	}
}
