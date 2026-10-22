package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// KeyRootInWorkTree is install.json's record that the user confirmed a store
// root inside a git work tree: the top directory of that work tree, as watch
// resolved it (Technical spec 7.2.3 step 5, 7.2.9, 7.2.10).
const KeyRootInWorkTree = "root_in_work_tree"

// WorkTreeTop returns the top directory of the git work tree dir sits in, or
// "" when it is in none. dir need not exist yet: its nearest existing
// ancestor is resolved through symlinks and the walk up for a ".git" entry,
// directory or file, starts from there, so the answer names the same
// directory purge finds once the root exists. Nothing is executed and
// nothing is written.
func WorkTreeTop(dir string) (string, error) {
	resolved, err := resolveExisting(dir)
	if err != nil {
		return "", err
	}
	for d := resolved; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", nil
		}
	}
}

// resolveExisting is dir with its nearest existing ancestor's symlinks
// resolved and the part below it, which does not exist yet, kept as given.
func resolveExisting(dir string) (string, error) {
	var rest []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		r, err := filepath.EvalSymlinks(d)
		if err == nil {
			return filepath.Join(append([]string{r}, rest...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(d) == d {
			return filepath.Clean(dir), nil
		}
		rest = append([]string{filepath.Base(d)}, rest...)
	}
}

// RecordedRootInWorkTree returns the work tree top that install.json under
// root records as confirmed, or "" when there is no install.json or it
// records none. A malformed install.json is an error, not "".
func RecordedRootInWorkTree(root string) (string, error) {
	rec, err := ReadInstallRecord(root)
	if errors.Is(err, ErrNoStore) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return rec.RootInWorkTree, nil
}

// SetRootInWorkTree records top as install.json's root_in_work_tree under
// root. Every other key is kept as it was read. The file is replaced by a
// temporary file beside it, a full sync and a rename, so a hook reading it
// sees the old record or the new one. A root with no install.json is
// ErrNoStore: this never creates one.
func SetRootInWorkTree(root, top string) error {
	return UpdateInstallRecord(root, func(r *InstallRecord) { r.RootInWorkTree = top })
}
