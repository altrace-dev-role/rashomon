//go:build !darwin

package store

import (
	"os"

	"github.com/altrace-dev-role/rashomon/internal/fault"
)

// barrierSync is f.Sync() everywhere but darwin. Linux and Windows have no
// ordering primitive cheaper than a full sync, so a deferred append on them
// costs what it always did, and Store.Flush's one full sync repeats work that
// is already done. The sync recorder logs it as the barrier it stands in for.
func barrierSync(f *os.File) error {
	fault.RecordSync(fault.SyncBarrier, f.Name())
	return f.Sync()
}
