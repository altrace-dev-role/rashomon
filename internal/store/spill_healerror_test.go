package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestHealAppendErrorCutFalseLeavesTheWriteOnDisk: appendLine passes
// cut=false for the spill file, because it is the only file a lockless
// writer ever targets (store.go's bestEffort branch, TestCutBackRaceErases
// AConcurrentAppend). With cut=false, a write that landed on disk but is
// still reported failed must stay exactly as it landed -- no cutBack call,
// no truncate -- because cutBack cannot tell that record apart from one a
// lockless writer put down in its own Stat-to-Truncate gap.
func TestHealAppendErrorCutFalseLeavesTheWriteOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spill")
	f := openForAppend(t, path)

	seed := []byte(`{"seed":true}` + "\n")
	if _, err := f.Write(seed); err != nil {
		t.Fatal(err)
	}
	size := int64(len(seed))

	line := []byte(`{"line":true}` + "\n")
	n, err := f.Write(line)
	if err != nil {
		t.Fatal(err)
	}

	gotErr := healAppendError(f, size, n, false, errors.New("reported failed"))
	if gotErr == nil {
		t.Fatal("want the write error back")
	}

	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	want := string(seed) + string(line)
	if string(got) != want {
		t.Errorf("healAppendError(cut=false) changed a write only reported failed: got %q, want %q", got, want)
	}
}

// TestHealAppendErrorCutTrueTruncates pins the other side: cut=true, the
// setting every ordered and locked append uses, still removes the bytes a
// failed write put down.
func TestHealAppendErrorCutTrueTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ordered")
	f := openForAppend(t, path)

	seed := []byte(`{"seed":true}` + "\n")
	if _, err := f.Write(seed); err != nil {
		t.Fatal(err)
	}
	size := int64(len(seed))

	line := []byte(`{"line":true}` + "\n")
	n, err := f.Write(line)
	if err != nil {
		t.Fatal(err)
	}

	if err := healAppendError(f, size, n, true, errors.New("reported failed")); err == nil {
		t.Fatal("want the write error back")
	}

	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != string(seed) {
		t.Errorf("healAppendError(cut=true) left the failed write on disk: got %q, want %q", got, seed)
	}
}
