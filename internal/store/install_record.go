package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// InstallRecord is install.json: the install's identity, and what watch
// found on the machine and changed on it, which detach and purge need to put
// it back (Technical spec 7.2.3, 7.2.9).
//
// Every field after SchemaVersion is optional. A store opened by a hook, or
// written by a release that did not record them, carries none of them, and a
// reader treats each missing one as "watch recorded nothing here" -- never as
// a value. A field this release does not know is kept when the record is
// rewritten.
type InstallRecord struct {
	InstallID       string `json:"install_id"`
	CreatedAtUnixMS int64  `json:"created_at_unix_ms"`
	SchemaVersion   int    `json:"schema_version"`

	// SettingsExisted says whether the user's settings.json existed when
	// watch first wrote it. False means watch created the file, and a detach
	// that restores the pre-image removes it.
	SettingsExisted *bool `json:"settings_existed,omitempty"`
	// PreSHA256 is the hex sha256 of settings.json before watch's first
	// write, the bytes detach must reproduce when the file is still the one
	// watch left. Empty when watch could not know them: the file already held
	// this install's entries and had changed since the watch that wrote them.
	PreSHA256 string `json:"pre_sha256,omitempty"`
	// PostSHA256 is the hex sha256 of settings.json as watch last wrote it.
	// The file still hashing to it is what "unchanged since watch" means.
	PostSHA256 string `json:"post_sha256,omitempty"`
	// CreatedDirs are the directories watch created, absolute, for purge to
	// remove once they are empty. A directory is added once and never
	// dropped by a later watch.
	CreatedDirs []string `json:"created_dirs,omitempty"`
	// RootInWorkTree is the top directory of the git work tree the store
	// root sits in, recorded only when the user confirmed that root at
	// watch. purge refuses a root inside a work tree without it.
	RootInWorkTree string `json:"root_in_work_tree,omitempty"`
}

// installRecordKeys are the keys InstallRecord owns. A rewrite replaces
// these and keeps every other key as it was read.
var installRecordKeys = []string{
	"install_id", "created_at_unix_ms", "schema_version",
	"settings_existed", "pre_sha256", "post_sha256", "created_dirs", "root_in_work_tree",
}

// ReadInstallRecord reads install.json under root. It returns ErrNoStore
// when there is none, and creates nothing either way.
func ReadInstallRecord(root string) (InstallRecord, error) {
	var rec InstallRecord
	b, err := os.ReadFile(filepath.Join(root, fileInstallMeta))
	if errors.Is(err, os.ErrNotExist) {
		return rec, ErrNoStore
	}
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(b, &rec); err != nil || rec.InstallID == "" {
		return rec, errors.New("store: install metadata is malformed")
	}
	return rec, nil
}

// UpdateInstallRecord applies edit to install.json under root and replaces
// the file atomically: a temporary file in the root, a full sync, then a
// rename, so a hook reading it concurrently sees the old record or the new
// one. The install id is not edit's to change, and a store with no
// install.json is ErrNoStore: this never creates one.
func UpdateInstallRecord(root string, edit func(*InstallRecord)) error {
	path := filepath.Join(root, fileInstallMeta)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoStore
	}
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	var rec InstallRecord
	if json.Unmarshal(b, &raw) != nil || json.Unmarshal(b, &rec) != nil || rec.InstallID == "" {
		return errors.New("store: install metadata is malformed")
	}
	id := rec.InstallID
	edit(&rec)
	rec.InstallID = id

	ours, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	var fresh map[string]json.RawMessage
	if err := json.Unmarshal(ours, &fresh); err != nil {
		return err
	}
	for _, k := range installRecordKeys {
		delete(raw, k)
	}
	for k, v := range fresh {
		raw[k] = v
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return replaceFile(path, append(encoded, '\n'))
}

// replaceFile writes content under a temporary name beside path, fully
// syncs it, and renames it over path.
func replaceFile(path string, content []byte) error {
	dir, base := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // a no-op after the rename; cleanup before it

	if err := tmp.Chmod(fileMode); err != nil {
		tmp.Close() //nolint:errcheck // the chmod error is what matters
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close() //nolint:errcheck // the write error is what matters
		return err
	}
	if err := syncFull(tmp); err != nil {
		tmp.Close() //nolint:errcheck // the sync error is what matters
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("store: replacing %s: %w", base, err)
	}
	return nil
}
