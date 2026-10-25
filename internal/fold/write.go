package fold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Write replaces path with f's JSON encoding atomically: a temporary file
// in the same directory, fsync, then rename -- the same shape
// internal/settings/write.go and internal/baseline use, for the same
// reason. At every instant the path holds either the previous fold.json or
// the complete new one; a process killed mid-write leaves the temporary
// file behind and the real path untouched.
//
// recap is fold.json's one writer (its own and Validate's doc; VERIFY's
// rule that recap alone writes it): nothing here takes a lock, because
// nothing else is ever racing this write.
//
// A document that would exceed MaxBytes is refused before anything is
// written or renamed: a fold that grew without bound would be the
// O(session) read it exists to replace (fold.go's doc on MaxBytes), so
// writing it anyway would make the cap a claim this package does not keep.
func Write(path string, f *File) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(b) > MaxBytes {
		return fmt.Errorf("fold: encoded file is %d bytes, over MaxBytes (%d)", len(b), MaxBytes)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, TempPrefix+"*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // best effort; a no-op after a successful rename

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close() //nolint:errcheck // the chmod error is what matters
		return err
	}
	if _, err := tmp.Write(b); err != nil {
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

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	// Make the rename itself durable. Best effort: some filesystems refuse
	// to sync a directory, and the rename has already happened.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()  //nolint:errcheck // best effort, as above
		_ = d.Close() //nolint:errcheck // a directory opened only to sync it holds nothing to lose
	}
	return nil
}
