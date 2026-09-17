package settings

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/altrace-attest/internal/fault"
)

// ErrChanged reports that the file changed between being read and being
// written. The caller reloads and re-applies; it never overwrites an edit it
// did not see.
var ErrChanged = errors.New("settings: file changed since it was read")

// Write replaces the file atomically: a temporary file in the same directory,
// fsync, then rename(2). At every instant the path holds either the previous
// content or the complete new content. There is no state in between for a
// crash to leave behind, and a crash in the temporary file leaves the real one
// untouched.
//
// fsync before the rename is required and is review-only: a process kill
// cannot verify it, because the page cache survives the process.
//
// original is what the caller read. If the file no longer matches it, nothing
// is written and ErrChanged is returned, so an edit Claude Code made in the
// meantime is not silently discarded.
func Write(path string, original, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".settings.json.attest-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// A no-op after a successful rename; cleanup after any failure before it.
	defer os.Remove(tmpPath) //nolint:errcheck // best effort

	fault.Inject(fault.PointSettingsOpened)

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close() //nolint:errcheck // the chmod error is what matters
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck // the write error is what matters
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close() //nolint:errcheck // the sync error is what matters
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if !bytes.Equal(current, original) {
		return ErrChanged
	}

	fault.Inject(fault.PointSettingsBeforeRename)

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	// Make the rename itself durable. Best effort: some filesystems refuse to
	// sync a directory, and the rename has already happened.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// ReadOriginal reads the file's current bytes for a later Write. An absent
// file reads as nil, which Write treats as "still absent".
func ReadOriginal(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
