package regfile

import (
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rows of the capped reader (card READ-SAFE1a, condition c3) that need
// no named pipe, device or link, so they run on every platform the Test job
// runs; regfile_capped_unix_test.go has the others. The pgraph module has
// its own form of this reader (pgraph/cmd/pgraph, readCapped) with its own
// rows (readcapped_test.go there): that one refuses a link, this one follows
// it, as Open does.

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
// matches ErrOverCap, carries the cap and a size over it, and names path.
func wantOverCap(t *testing.T, err error, path string, max, size int64) {
	t.Helper()
	if !errors.Is(err, ErrOverCap) {
		t.Fatalf("error %v, want one matching ErrOverCap", err)
	}
	var oc *OverCapError
	if !errors.As(err, &oc) || oc.Max != max || oc.Size != size {
		t.Fatalf("error %v: OverCapError %+v, want Max %d and Size %d", err, oc, max, size)
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Path != path {
		t.Fatalf("error %v does not name the path %s", err, path)
	}
}

// TestRS1a_C3_ReadCappedReadsUpToTheCap: a regular file of exactly max bytes,
// and an empty one at max 0, read whole.
func TestRS1a_C3_ReadCappedReadsUpToTheCap(t *testing.T) {
	for _, n := range []int{0, 1, 4096} {
		p := readCappedFile(t, n)
		b, err := ReadCapped(p, int64(n))
		if err != nil || len(b) != n {
			t.Errorf("ReadCapped of %d bytes at max %d = %d bytes, %v; want all of it", n, n, len(b), err)
		}
	}
}

// TestRS1a_C3_ReadCappedRefusesOneByteOver: a file one byte over max is
// refused by its size before a byte is read, with no bytes returned.
func TestRS1a_C3_ReadCappedRefusesOneByteOver(t *testing.T) {
	p := readCappedFile(t, 4097)
	b, err := ReadCapped(p, 4096)
	if b != nil {
		t.Errorf("ReadCapped returned %d bytes with the refusal", len(b))
	}
	wantOverCap(t, err, p, 4096, 4097)
}

// TestRS1a_C3_ReadCappedAtTheLargestCap: max+1 must not overflow. A reader
// built on io.LimitReader(f, max+1) reads nothing at math.MaxInt64, since
// the limit wraps negative, and returns an empty file with no error.
func TestRS1a_C3_ReadCappedAtTheLargestCap(t *testing.T) {
	p := readCappedFile(t, 3)
	b, err := ReadCapped(p, math.MaxInt64)
	if err != nil || string(b) != "xxx" {
		t.Errorf("ReadCapped at max MaxInt64 = %q, %v; want \"xxx\"", b, err)
	}
}

// TestRS1a_C3_ReadCappedRefusesWhatIsNotARegularFile: a directory is
// refused as not regular and a missing path is the open's own error, as
// with Open.
func TestRS1a_C3_ReadCappedRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	if b, err := ReadCapped(dir, 1<<20); b != nil || !errors.Is(err, ErrNotRegular) {
		t.Errorf("ReadCapped(a directory) = %d bytes, %v; want ErrNotRegular", len(b), err)
	}
	if b, err := ReadCapped(filepath.Join(dir, "none"), 1<<20); b != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadCapped(missing) = %d bytes, %v; want fs.ErrNotExist", len(b), err)
	}
	if rc, err := OpenCapped(dir, 1<<20); rc != nil || !errors.Is(err, ErrNotRegular) {
		t.Errorf("OpenCapped(a directory) = %v, %v; want ErrNotRegular", rc, err)
	}
}

// TestRS1a_C3_OpenCappedRefusesOverTheCapAtTheOpen: OpenCapped refuses a
// file over max by its size, and returns no reader.
func TestRS1a_C3_OpenCappedRefusesOverTheCapAtTheOpen(t *testing.T) {
	p := readCappedFile(t, 11)
	rc, err := OpenCapped(p, 10)
	if rc != nil {
		rc.Close() //nolint:errcheck // read-only
		t.Errorf("OpenCapped returned a reader with the refusal")
	}
	wantOverCap(t, err, p, 10, 11)
}

// TestRS1a_C3_OpenCappedRefusesAFileThatGrowsPastTheCap: a file at max when
// it is opened, then grown by one byte, is refused on the read of byte
// max+1, never read on to its end and never taken for a short file. The
// error's Size is the max+1 bytes read before the refusal.
func TestRS1a_C3_OpenCappedRefusesAFileThatGrowsPastTheCap(t *testing.T) {
	p := readCappedFile(t, 10)
	rc, err := OpenCapped(p, 10)
	if err != nil {
		t.Fatalf("OpenCapped of a file at the cap: %v", err)
	}
	defer rc.Close() //nolint:errcheck // read-only
	w, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("yz"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(rc)
	if len(b) > 10 {
		t.Errorf("read %d bytes past a 10 byte cap", len(b))
	}
	wantOverCap(t, err, p, 10, 11)
}

// TestRS1a_C3_OpenCappedReadsAFileAtTheCap: the reader of a file at max
// gives every byte and then io.EOF, so a decoder sees a whole file.
func TestRS1a_C3_OpenCappedReadsAFileAtTheCap(t *testing.T) {
	p := readCappedFile(t, 10)
	rc, err := OpenCapped(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close() //nolint:errcheck // read-only
	if b, err := io.ReadAll(rc); err != nil || len(b) != 10 {
		t.Errorf("ReadAll = %d bytes, %v; want 10 and no error", len(b), err)
	}
}
