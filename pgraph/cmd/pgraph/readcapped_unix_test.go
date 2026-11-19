//go:build unix

package main

import (
	"os"
	"testing"
)

// The named-pipe row of pgraph's capped reader (card READ-SAFE1a, condition
// c3); readcapped_test.go has the others, and internal/regfile's
// regfile_capped_unix_test.go is the twin of the root module's form.

// TestRS1a_C3_PgraphReadCappedAtRefusesAPipeSwappedInAfterTheLstat: the
// name is Lstat-ed as a regular file, then a named pipe with no writer takes
// its place before the open. The open must not wait for a writer, and the
// pipe is refused. A plain open would hold the sync here, so the Lstat alone
// does not keep a pipe out.
func TestRS1a_C3_PgraphReadCappedAtRefusesAPipeSwappedInAfterTheLstat(t *testing.T) {
	p := readCappedFile(t, 3)
	lfi := mustLstat(t, p)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	readSafeReturnsInside(t, p, func(p string) {
		if b, err := readCappedAt(p, 1<<20, lfi); b != nil || err == nil {
			t.Errorf("readCappedAt of a pipe swapped in = %q, %v; want a refusal", b, err)
		}
	})
}
