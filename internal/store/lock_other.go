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
func LockFile(*os.File, time.Duration) (func(), error) {
	return nil, errors.New("store: file locking is not implemented on this platform")
}
