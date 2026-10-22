package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// fileCodexCreated records that watch --harness codex created Codex's
// hooks.json, and which path it created. detach --harness codex deletes the
// file only when this names it and nothing but our entries was in it.
//
// It is read and removed by path, never through Open: detach runs on a
// machine whose store may be gone, and must not create one by looking.
const fileCodexCreated = "codex-hooks-created"

// MarkCodexHooksCreated records that this install created the hooks.json at
// path.
func MarkCodexHooksCreated(root, path string) error {
	return writeSmall(filepath.Join(root, fileCodexCreated), []byte(path+"\n"))
}

// CodexHooksCreated reports whether the store at root records creating the
// hooks.json at path. A missing or unreadable marker is false: deleting a
// file on evidence this program cannot read would delete one it may not have
// made.
func CodexHooksCreated(root, path string) bool {
	data, err := os.ReadFile(filepath.Join(root, fileCodexCreated))
	return err == nil && strings.TrimSuffix(string(data), "\n") == path
}

// ClearCodexHooksCreated forgets that hooks.json was created here. A marker
// already gone is not an error.
func ClearCodexHooksCreated(root string) error {
	err := os.Remove(filepath.Join(root, fileCodexCreated))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
