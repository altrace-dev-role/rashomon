package main

import (
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rows of pgraph's capped reader (card READ-SAFE1a, condition c3) that
// need no named pipe; readcapped_unix_test.go has that one. The root module
// has its own form of this reader (internal/regfile, ReadCapped and
// OpenCapped) with its own rows: that one follows a link, this one refuses
// any link, since the vouch read of hook.go must not read through one.

// readCappedFile writes n bytes of 'x' to a fresh file and returns its path.
func readCappedFile(t *testing.T, n int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", n)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// wantOverCap fails t unless err is an over-cap refusal of path at max: it
// matches errOverCap, carries the cap and a size over it, and names path.
func wantOverCap(t *testing.T, err error, path string, max, size int64) {
	t.Helper()
	if !errors.Is(err, errOverCap) {
		t.Fatalf("error %v, want one matching errOverCap", err)
	}
	var oc *overCapError
	if !errors.As(err, &oc) || oc.Max != max || oc.Size != size {
		t.Fatalf("error %v: overCapError %+v, want Max %d and Size %d", err, oc, max, size)
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Path != path {
		t.Fatalf("error %v does not name the path %s", err, path)
	}
}

// TestRS1a_C3_PgraphReadCappedReadsUpToTheCap: a regular file of exactly
// max bytes, and an empty one at max 0, read whole.
func TestRS1a_C3_PgraphReadCappedReadsUpToTheCap(t *testing.T) {
	for _, n := range []int{0, 1, 4096} {
		p := readCappedFile(t, n)
		b, err := readCapped(p, int64(n))
		if err != nil || len(b) != n {
			t.Errorf("readCapped of %d bytes at max %d = %d bytes, %v; want all of it", n, n, len(b), err)
		}
	}
}

// TestRS1a_C3_PgraphReadCappedRefusesOneByteOver: a file one byte over max
// is refused by its size with no bytes, as an over-cap error that readLog
// can tell from a file that does not decode.
func TestRS1a_C3_PgraphReadCappedRefusesOneByteOver(t *testing.T) {
	p := readCappedFile(t, 4097)
	b, err := readCapped(p, 4096)
	if b != nil {
		t.Errorf("readCapped returned %d bytes with the refusal", len(b))
	}
	wantOverCap(t, err, p, 4096, 4097)
}

// TestRS1a_C3_PgraphReadCappedAtTheLargestCap: max+1 must not overflow; see
// the twin row of regfile.
func TestRS1a_C3_PgraphReadCappedAtTheLargestCap(t *testing.T) {
	p := readCappedFile(t, 3)
	b, err := readCapped(p, math.MaxInt64)
	if err != nil || string(b) != "xxx" {
		t.Errorf("readCapped at max MaxInt64 = %q, %v; want \"xxx\"", b, err)
	}
}

// TestRS1a_C3_PgraphReadCappedRefusesALinkAndADirectory: a link, even to a
// regular file under the cap, is refused, and so is a directory; neither is
// an over-cap refusal. A missing path is the Lstat's own error, so readLog
// still reads no log as an empty one.
func TestRS1a_C3_PgraphReadCappedRefusesALinkAndADirectory(t *testing.T) {
	file := readCappedFile(t, 3)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
	for _, p := range []string{link, filepath.Dir(file)} {
		if b, err := readCapped(p, 1<<20); b != nil || err == nil || errors.Is(err, errOverCap) {
			t.Errorf("readCapped(%s) = %d bytes, %v; want a refusal that is not over-cap", p, len(b), err)
		}
	}
	if _, err := readCapped(filepath.Join(t.TempDir(), "none"), 1<<20); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("readCapped(missing) = %v, want fs.ErrNotExist", err)
	}
}

// TestRS1a_C3_PgraphReadCappedAtRefusesASwapAfterTheLstat: the name is
// Lstat-ed as one regular file, then another is renamed over it before the
// open. The descriptor's file is not the one the Lstat saw, so the read is
// refused, not taken from the new file.
func TestRS1a_C3_PgraphReadCappedAtRefusesASwapAfterTheLstat(t *testing.T) {
	p := readCappedFile(t, 3)
	lfi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(p), "other")
	if err := os.WriteFile(other, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other, p); err != nil {
		t.Fatal(err)
	}
	if b, err := readCappedAt(p, 1<<20, lfi); b != nil || err == nil {
		t.Errorf("readCappedAt after a swap = %q, %v; want a refusal", b, err)
	}
	if b, err := readCappedAt(p, 1<<20, mustLstat(t, p)); err != nil || string(b) != "new" {
		t.Errorf("readCappedAt with the Lstat of the file now there = %q, %v; want \"new\"", b, err)
	}
}

// TestRS1a_C3_PgraphReadAllCappedRefusesPastTheCap: the read itself stops
// at byte max+1, so a file that grew after its fstat is refused and no byte
// past max reaches the caller. The error's Size is the max+1 bytes read.
func TestRS1a_C3_PgraphReadAllCappedRefusesPastTheCap(t *testing.T) {
	b, err := readAllCapped(strings.NewReader("0123456789yz"), "grown", 10)
	if len(b) > 10 {
		t.Errorf("read %d bytes past a 10 byte cap", len(b))
	}
	if b != nil {
		t.Errorf("readAllCapped returned %d bytes with the refusal", len(b))
	}
	wantOverCap(t, err, "grown", 10, 11)
	if b, err := readAllCapped(strings.NewReader("0123456789"), "at", 10); err != nil || len(b) != 10 {
		t.Errorf("readAllCapped at the cap = %d bytes, %v; want 10 and no error", len(b), err)
	}
}

// mustLstat is the Lstat of p.
func mustLstat(t *testing.T, p string) fs.FileInfo {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}
