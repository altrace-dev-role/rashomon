package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestCutBackRaceErasesAConcurrentAppend pins the hazard cutBack's own doc
// now names: between its Stat and its Truncate there is nothing excluding
// a writer with no lock on f, so a cut-back lands on whatever is there by
// the time Truncate runs, not by the time Stat did. cutBackSync makes that
// window land a write deterministically, in place of a real race, and the
// write is gone -- which is exactly why appendLine never lets this run
// against the spill file (its only lockless writer), passing cut=false
// there instead of calling cutBack at all.
func TestCutBackRaceErasesAConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spill")
	f := openForAppend(t, path)

	seed := []byte(`{"seed":true}` + "\n")
	if _, err := f.Write(seed); err != nil {
		t.Fatal(err)
	}
	size := int64(len(seed))

	landed := make(chan struct{})
	cutBackSync = func() {
		g, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, fileMode)
		if err != nil {
			t.Error(err)
			close(landed)
			return
		}
		if _, err := g.Write([]byte(`{"concurrent":true}` + "\n")); err != nil {
			t.Error(err)
		}
		if err := g.Close(); err != nil {
			t.Error(err)
		}
		close(landed)
	}
	t.Cleanup(func() { cutBackSync = nil })

	line := []byte(`{"line":true}` + "\n")
	n, werr := f.Write(line)
	if werr != nil {
		t.Fatalf("seeding the failed write: %v", werr)
	}
	if err := cutBack(f, size, n); err != nil {
		t.Fatal(err)
	}
	<-landed

	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if bytes.Contains(got, []byte(`"concurrent":true`)) {
		t.Fatalf("premise broken: cutBack no longer erases a write landed in its own gap: %s", got)
	}
}
