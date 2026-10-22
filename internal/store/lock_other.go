//go:build !unix && !windows

package store

import (
	"errors"
	"os"
	"time"
)

// LockFile refuses rather than pretending. Without an advisory lock, concurrent
// appends of records larger than a pipe buffer interleave, and a store that
// silently corrupts itself under concurrency is worse than one that will not
// open at all.
func lockFileWait(*os.File, time.Duration) (func(), time.Duration, error) {
	return nil, 0, errors.New("store: file locking is not implemented on this platform")
}

// readRefused reports whether a read failed because another handle holds
// the file's lock. With no lock on this platform, none ever does.
func readRefused(error) bool { return false }
