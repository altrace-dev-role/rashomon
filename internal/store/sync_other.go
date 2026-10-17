//go:build !darwin

package store

import (
	"os"

	"github.com/altrace-dev-role/rashomon/internal/fault"
)

// barrierSync is f.Sync() everywhere but darwin. Linux and Windows have no
// ordering primitive cheaper than a full sync, so a deferred append on them
// costs what it always did and is durable when it returns: barrierSync says
// so, and Store.Flush, which would only repeat it, has nothing left to sync.
// The sync recorder logs it as the barrier it stands in for.
func barrierSync(f *os.File) (durable bool, err error) {
	fault.RecordSync(fault.SyncBarrier, f.Name())
	if err := f.Sync(); err != nil {
		return false, err
	}
	return true, nil
}
