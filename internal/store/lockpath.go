package store

import "os"

// LockPath takes the store's exclusive lock on the file at path, creating it
// 0600 if it is absent, waiting at most the store's lock budget, and returns
// ErrLockTimeout when that runs out.
//
// It is for a caller beside the store that does a load-modify-save of a file
// of its own: the project baseline, which a report render and a `forget
// --host` both rewrite. The path is a sidecar lock file, not the data file,
// because such a save renames a new file over the data file, and a lock held
// on the inode it replaced would guard nothing. release unlocks and closes.
func LockPath(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	unlock, err := lockCounted(f, lockBudget)
	if err != nil {
		f.Close() //nolint:errcheck // the lock error is what matters
		return nil, err
	}
	return func() {
		unlock()
		f.Close() //nolint:errcheck // a lock file: nothing was written to it
	}, nil
}
