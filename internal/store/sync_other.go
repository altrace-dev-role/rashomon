//go:build !darwin

package store

import "os"

// barrierSync is f.Sync() everywhere but darwin. Linux and Windows have no
// ordering primitive cheaper than a full sync, so a deferred append on them
// costs what it always did, and Store.Flush's one full sync repeats work that
// is already done.
func barrierSync(f *os.File) error { return f.Sync() }
