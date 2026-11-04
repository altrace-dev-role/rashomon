//go:build !unix

package main

import "context"

// lockRoot takes no lock where flock is not. Two syncs of one root at once
// may then leave digests beside a log they were not written with, until the
// next sync (each file is still renamed whole).
func lockRoot(ctx context.Context, root string) (unlock func(), err error) {
	return func() {}, nil
}
