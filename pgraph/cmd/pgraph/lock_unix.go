//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// lockRoot takes an exclusive flock on the directory root itself, waiting
// for another sync of root to end until ctx does (review item 2 on #230).
// Locking the directory, not a file in .pgraph, writes nothing, so a sync
// that fails still leaves no .pgraph behind. The kernel drops a flock when
// its holder exits, a timeout's kill included, so a killed sync never stops
// the next one, where a lock file made with O_EXCL would outlive it.
func lockRoot(ctx context.Context, root string) (unlock func(), err error) {
	f, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, errLocked
		case <-time.After(50 * time.Millisecond):
		}
	}
}
