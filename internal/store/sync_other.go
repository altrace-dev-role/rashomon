//go:build !darwin

package store

import (
	"os"
)

// barrierSync is f.Sync() everywhere but darwin. Linux and Windows have no
// ordering primitive cheaper than a full sync, so a deferred append on them
// costs what it always did and is durable when it returns: barrierSync says
// so, and Store.Flush, which would only repeat it, has nothing left to sync.
// The sync recorder logs it as SyncBarrierFull, not SyncBarrier: it is a
// full sync, run inside the append's lock, and logging it as the cheap
// ordering darwin's real barrier does would let an acceptance test's "no
// full sync inside a lock" pass here by construction, with the cost just
// mislabeled rather than absent.
func barrierSync(f *os.File) (durable bool, err error) {
	if err := syncFullAsBarrier(f); err != nil {
		return false, err
	}
	return true, nil
}
