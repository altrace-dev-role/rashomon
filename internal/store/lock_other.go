//go:build !unix

package store

import (
	"errors"
	"os"
	"time"
)

const lockBudget = 2 * time.Second

// lockFile refuses rather than pretending. Without an advisory lock, concurrent
// appends of records larger than a pipe buffer interleave, and a store that
// silently corrupts itself under concurrency is worse than one that will not
// open at all.
func lockFile(*os.File, time.Duration) (func(), error) {
	return nil, errors.New("store: file locking is not implemented on this platform")
}
