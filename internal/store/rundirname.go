package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// RunDirName is the name of the directory under runs/ that holds a session's
// records: the session id when it is a plain path element, a digest of it
// otherwise. It is a pure function of the id, so a reader can ask where a run
// would be without opening the store, which creates and chmods what it opens.
func RunDirName(sessionID string) string { return segment(sessionID) }

// RunExistsAt reports whether root holds a run for sessionID. It only looks:
// it opens nothing, creates nothing and changes nothing under root, so it is
// safe to ask of a store that is not the caller's to write. A root with no
// runs directory holds no run; any other failure to look is returned.
func RunExistsAt(root, sessionID string) (bool, error) {
	fi, err := os.Lstat(filepath.Join(root, DirRuns, RunDirName(sessionID)))
	switch {
	case err == nil:
		return fi.IsDir(), nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}
