package regfile

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// countedReader is r, counting the Read calls it is given.
type countedReader struct {
	r     io.Reader
	reads int
}

func (c *countedReader) Read(p []byte) (int, error) {
	c.reads++
	return c.r.Read(p)
}

// TestWS1_RB3_ReadBoundedRefusesAStatedSizeOverTheBoundBeforeAnyRead: a
// stated size one byte over limit is ErrOverBound, with no byte returned
// and no Read call made, even where the reader holds fewer bytes than limit
// (the file shrank after its fstat); a stated size at limit is read. Break:
// drop the check on the stated size, so the read loop's check is the only
// bound.
func TestWS1_RB3_ReadBoundedRefusesAStatedSizeOverTheBoundBeforeAnyRead(t *testing.T) {
	const limit = 64
	over := &countedReader{r: bytes.NewReader([]byte("eight by"))}
	b, err := ReadBounded(over, limit+1, limit)
	if !errors.Is(err, ErrOverBound) || b != nil || over.reads != 0 {
		t.Errorf("stated size %d over limit %d: ReadBounded = %q, %v after %d reads; want nil, ErrOverBound after 0",
			limit+1, limit, b, err, over.reads)
	}
	full := bytes.Repeat([]byte("x"), limit)
	b, err = ReadBounded(bytes.NewReader(full), limit, limit)
	if err != nil || !bytes.Equal(b, full) {
		t.Errorf("stated size %d at limit: ReadBounded = %d bytes, %v; want the %d bytes", limit, len(b), err, limit)
	}
}

// TestWS1_RB5_ReadBoundedRefusesAReaderLongerThanItsStatedSize: a reader
// holding limit+1 bytes behind a stated size of 10 (a file that grew after
// its fstat) is ErrOverBound with no byte returned; the same reader at
// limit bytes is read whole. limit is over 512, so the read runs past the
// first buffer's end. Break: drop the bound check in the read loop, so only
// the stated size is checked.
func TestWS1_RB5_ReadBoundedRefusesAReaderLongerThanItsStatedSize(t *testing.T) {
	const limit, stated = 1000, 10
	for _, c := range []struct {
		n    int
		over bool
	}{{limit + 1, true}, {limit, false}} {
		in := bytes.Repeat([]byte("y"), c.n)
		b, err := ReadBounded(bytes.NewReader(in), stated, limit)
		switch {
		case c.over && (!errors.Is(err, ErrOverBound) || b != nil):
			t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadBounded = %d bytes, %v; want nil, ErrOverBound",
				c.n, stated, limit, len(b), err)
		case !c.over && (err != nil || !bytes.Equal(b, in)):
			t.Errorf("%d bytes behind a stated size of %d, limit %d: ReadBounded = %d bytes, %v; want the %d bytes",
				c.n, stated, limit, len(b), err, c.n)
		}
	}
}
