//go:build !darwin

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestREC34l_TheStandInBarrierIsDurable: off darwin the barrier is a full
// sync, so an append it covered is durable when it returns and the group
// commit's Flush has nothing left to sync (P09-F9).
func TestREC34l_TheStandInBarrierIsDurable(t *testing.T) {
	f, err := os.OpenFile(filepath.Join(t.TempDir(), FileRecords), appendFlags, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	durable, err := barrierSync(f)
	if err != nil {
		t.Fatalf("barrierSync on a regular file: %v", err)
	}
	if !durable {
		t.Error("the stand-in barrier, a full sync, reported its write not durable: Flush would sync it a second time")
	}
}
