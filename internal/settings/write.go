package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/fault"
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
	// Follow a symlink to its target before choosing what to replace.
	//
	// rename(2) replaces the LINK, not the file it points at, so a
	// dotfile-managed settings.json -- stow, chezmoi, yadm, all common among
	// the people who would run this -- was silently turned into a regular
	// file. The managed file stopped being the one Claude Code reads, kept
	// its stale contents, and detach could not put the link back because by
	// then there was no link to restore.
	//
	// Only a path with nothing at it is the ordinary first install. A link
	// that does not resolve is refused: treating its failed resolution as
	// "does not exist yet" renamed over the link and caused the very bug this
	// comment describes.
	path, err := writeTarget(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".settings.json.rashomon-*")
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
		_ = d.Sync()  // best effort, as above: the rename already happened
		_ = d.Close() // a directory opened only to sync it holds nothing to lose
	}
	return nil
}

// writeTarget is the file Write replaces for path: path itself when nothing
// is there yet, or what path resolves to through any symlinks. A symlink that
// does not resolve is an error naming the link and its target, because
// writing anything -- over the link, or at a target outside the directory
// that holds it -- would decide for the user which of the two they meant.
func writeTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return "", err
	}
	return "", danglingLink(path, err)
}

// danglingLink describes a settings symlink that does not resolve.
func danglingLink(path string, cause error) error {
	target, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("settings: %s is a symlink that does not resolve (%v); not writing it", path, cause)
	}
	return fmt.Errorf("settings: %s is a symlink to %s, which does not resolve (%v); "+
		"not writing it -- restore the target, or replace the link with a file", path, target, cause)
}

// Remove deletes the file, under the same rule as Write: if it no longer holds
// original, nothing is removed and ErrChanged is returned. It is how detach
// returns a file watch created to not existing, which is the only way to
// restore the bytes of a file that had none.
func Remove(path string, original []byte) error {
	current, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return ErrChanged
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	// Make the removal durable, best effort, as Write does for its rename.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		d.Sync()  //nolint:errcheck // some filesystems refuse a directory sync
		d.Close() //nolint:errcheck // read-only handle
	}
	return nil
}

// ReadState reads the file's current bytes and whether it exists at all. An
// empty file and an absent one read alike through ReadOriginal; watch has to
// record which it found, because detach restores them differently. A symlink
// that does not resolve is an error, as it is for ReadOriginal: read as
// absent, it would let detach report nothing to detach and watch go on to
// write.
func ReadState(path string) (data []byte, existed bool, err error) {
	if _, err := writeTarget(path); err != nil {
		return nil, false, err
	}
	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// ReadOriginal reads the file's current bytes for a later Write. An absent
// file reads as nil, which Write treats as "still absent". A symlink that
// does not resolve is an error, not an absent file: reading it as empty
// would let detach report "nothing to detach" for a file it never saw, and
// let watch go on to write.
func ReadOriginal(path string) ([]byte, error) {
	data, _, err := ReadState(path)
	return data, err
}
