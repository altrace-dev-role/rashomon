package regfile

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
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

// offset is where f's next read starts: 0 while no read has been made, and
// the count of bytes taken from f after reads from its start.
func offset(t *testing.T, f *os.File) int64 {
	t.Helper()
	at, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// sizedInfo is a file's own file info with the size its fstat reported
// before the file changed: the mode is the file's, the size is n.
type sizedInfo struct {
	fs.FileInfo
	n int64
}

func (s sizedInfo) Size() int64 { return s.n }

// sizedAs is f's own file info with its size read as n, a file that grew or
// shrank after its fstat.
func sizedAs(t *testing.T, f *os.File, n int64) fs.FileInfo {
	t.Helper()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return sizedInfo{fi, n}
}

// TestWS1_RB3_ReadCappedFromRefusesAStatedSizeOverTheCapBeforeAnyRead: a
// stated size one byte over limit is an over-cap refusal naming the path,
// a *fs.PathError with Op "open", with no byte returned and the file's
// offset still 0, so no read was made, even where the file holds fewer
// bytes than limit (it shrank after its fstat); a stated size at limit is
// read. Break: drop the check on the stated size, so the capped reader's
// check is the only bound; or name that refusal by Op "read".
func TestWS1_RB3_ReadCappedFromRefusesAStatedSizeOverTheCapBeforeAnyRead(t *testing.T) {
	const limit = 64
	f, p := boundFile(t, []byte("eight by"))
	b, err := ReadCappedFrom(f, p, sizedAs(t, f, limit+1), limit)
	if at := offset(t, f); b != nil || at != 0 {
		t.Errorf("stated size %d over limit %d: ReadCappedFrom = %q at offset %d; want nil at offset 0",
			limit+1, limit, b, at)
	}
	wantOverCap(t, err, p, limit, limit+1)
	if pe, ok := err.(*fs.PathError); !ok || pe.Op != "open" {
		t.Errorf("stated size %d over limit %d: ReadCappedFrom = %#v; want a *fs.PathError with Op \"open\"",
			limit+1, limit, err)
	}
	full := bytes.Repeat([]byte("x"), limit)
	f, p = boundFile(t, full)
	b, err = ReadCappedFrom(f, p, sizedAs(t, f, limit), limit)
	if err != nil || !bytes.Equal(b, full) {
		t.Errorf("stated size %d at limit: ReadCappedFrom = %d bytes, %v; want the %d bytes", limit, len(b), err, limit)
	}
}

// TestWS1_RB5_ReadCappedFromRefusesAFileLongerThanItsStatedSize: a file of
// limit+1 bytes, and one of 64 times limit, behind a stated size of 10 (a
// file that grew after its fstat) are each an over-cap refusal at byte
// limit+1, a *fs.PathError with Op "read", with no byte returned, and
// exactly limit+1 bytes taken from the file, read by its offset: the
// reader asks the file for no more than one byte past limit, so the long
// file is not read on toward its end before the refusal. The same file at
// limit bytes is read whole. limit is over 512, so the read runs past the
// first buffer's end. Break: read around the capped reader, so only the
// stated size is checked; or ask the file for a whole buffer past the cap,
// so more than limit+1 bytes are taken before the refusal; or name the
// refusal by Op "open".
func TestWS1_RB5_ReadCappedFromRefusesAFileLongerThanItsStatedSize(t *testing.T) {
	const limit, stated = 1000, 10
	for _, c := range []struct {
		n    int
		over bool
	}{{64 * limit, true}, {limit + 1, true}, {limit, false}} {
		in := bytes.Repeat([]byte("y"), c.n)
		f, p := boundFile(t, in)
		b, err := ReadCappedFrom(f, p, sizedAs(t, f, stated), limit)
		if c.over {
			if at := offset(t, f); b != nil || at != limit+1 {
				t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadCappedFrom returned %d bytes and took %d from the file; want none returned and %d taken",
					c.n, stated, limit, len(b), at, limit+1)
			}
			wantOverCap(t, err, p, limit, limit+1)
			if pe, ok := err.(*fs.PathError); !ok || pe.Op != "read" {
				t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadCappedFrom = %#v; want a *fs.PathError with Op \"read\"",
					c.n, stated, limit, err)
			}
			continue
		}
		if err != nil || !bytes.Equal(b, in) {
			t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadCappedFrom = %d bytes, %v; want the %d bytes",
				c.n, stated, limit, len(b), err, c.n)
		}
	}
}

// TestWS1_RB7_ReadCappedFromRefusesWhatIsNotARegularFile: a directory handed
// in with its own file info is refused as not regular before any read, a
// *fs.PathError (Op "open") of the given path wrapping ErrNotRegular, with no
// byte returned: the check is the reader's own, not left to its caller. A
// directory stands in for a named pipe, which the windows build cannot make.
// Break: drop the mode check, so the read is tried on whatever the caller
// opened.
func TestWS1_RB7_ReadCappedFromRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReadCappedFrom(f, dir, fi, 1<<20)
	var pe *fs.PathError
	if b != nil || !errors.As(err, &pe) || pe.Op != "open" || pe.Path != dir || !errors.Is(err, ErrNotRegular) {
		t.Errorf("ReadCappedFrom(a directory) = %d bytes, %#v; want no byte and a *fs.PathError (open %s) wrapping ErrNotRegular",
			len(b), err, dir)
	}
}
