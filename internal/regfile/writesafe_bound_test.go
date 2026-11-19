package regfile

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// boundFile writes b to a fresh file and returns it open for reading, with
// its path; the file is closed before its directory is removed.
func boundFile(t *testing.T, b []byte) (*os.File, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bound")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // read-only
	return f, p
}

// offset is where f's next read starts: 0 while no read has been made.
func offset(t *testing.T, f *os.File) int64 {
	t.Helper()
	at, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// TestWS1_RB3_ReadCappedFromRefusesAStatedSizeOverTheCapBeforeAnyRead: a
// stated size one byte over limit is an over-cap refusal naming the path,
// with no byte returned and the file's offset still 0, so no read was made,
// even where the file holds fewer bytes than limit (it shrank after its
// fstat); a stated size at limit is read. Break: drop the check on the
// stated size, so the capped reader's check is the only bound.
func TestWS1_RB3_ReadCappedFromRefusesAStatedSizeOverTheCapBeforeAnyRead(t *testing.T) {
	const limit = 64
	f, p := boundFile(t, []byte("eight by"))
	b, err := ReadCappedFrom(f, p, limit+1, limit)
	if at := offset(t, f); b != nil || at != 0 {
		t.Errorf("stated size %d over limit %d: ReadCappedFrom = %q at offset %d; want nil at offset 0",
			limit+1, limit, b, at)
	}
	wantOverCap(t, err, p, limit, limit+1)
	full := bytes.Repeat([]byte("x"), limit)
	f, p = boundFile(t, full)
	b, err = ReadCappedFrom(f, p, limit, limit)
	if err != nil || !bytes.Equal(b, full) {
		t.Errorf("stated size %d at limit: ReadCappedFrom = %d bytes, %v; want the %d bytes", limit, len(b), err, limit)
	}
}

// TestWS1_RB5_ReadCappedFromRefusesAFileLongerThanItsStatedSize: a file of
// limit+1 bytes behind a stated size of 10 (a file that grew after its
// fstat) is an over-cap refusal at byte limit+1 with no byte returned; the
// same file at limit bytes is read whole. limit is over 512, so the read
// runs past the first buffer's end. Break: read around the capped reader,
// so only the stated size is checked.
func TestWS1_RB5_ReadCappedFromRefusesAFileLongerThanItsStatedSize(t *testing.T) {
	const limit, stated = 1000, 10
	for _, c := range []struct {
		n    int
		over bool
	}{{limit + 1, true}, {limit, false}} {
		in := bytes.Repeat([]byte("y"), c.n)
		f, p := boundFile(t, in)
		b, err := ReadCappedFrom(f, p, stated, limit)
		if c.over {
			if b != nil {
				t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadCappedFrom returned %d bytes with its refusal",
					c.n, stated, limit, len(b))
			}
			wantOverCap(t, err, p, limit, limit+1)
			continue
		}
		if err != nil || !bytes.Equal(b, in) {
			t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadCappedFrom = %d bytes, %v; want the %d bytes",
				c.n, stated, limit, len(b), err, c.n)
		}
	}
}
