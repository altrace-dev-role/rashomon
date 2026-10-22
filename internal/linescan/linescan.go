// Package linescan reads newline-delimited files a line at a time, as
// bufio.Scanner does with ScanLines, except that a line longer than the
// scanner's bound is stepped over and counted instead of ending the read.
//
// bufio.Scanner stops at the first line over its bound with
// bufio.ErrTooLong, and everything after that line is lost to the reader. A
// file another program writes, a Claude Code transcript or a nono trail, can
// hold such a line with ordinary lines after it, and those lines are the
// ones the reader is for.
package linescan

import (
	"bufio"
	"errors"
	"io"
)

// Scanner reads lines from an io.Reader. A line over the bound is one step of
// the scan, so a reader that keys state on line order sees where it was:
// Scan returns true, TooLong reports it, and Bytes is empty.
type Scanner struct {
	r *bufio.Reader
	// max is bufio.Scanner's bound: it counts the newline, so the longest
	// line read is max-1 bytes.
	max     int
	buf     []byte // a line assembled from fragments, never past max bytes
	line    []byte // the current line, in buf or in r's buffer
	tooLong bool
	skipped int
	done    bool
	err     error
}

// New returns a Scanner over r. initial is the read buffer, and a line that
// fits in it is handed back without a copy; a longer one is assembled up to
// max bytes and no further, so the memory a scan holds is bounded by max
// whatever the input.
func New(r io.Reader, initial, max int) *Scanner {
	return &Scanner{r: bufio.NewReaderSize(r, initial), max: max}
}

// Scan advances to the next line. It returns false at the end of the input
// or at a read error, which Err then returns; the partial line a read error
// cut short is not handed back.
func (s *Scanner) Scan() bool {
	s.line, s.tooLong = nil, false
	if s.done {
		return false
	}
	s.buf = s.buf[:0]
	n := 0 // bytes of this line read so far, its newline included
	for {
		frag, err := s.r.ReadSlice('\n')
		n += len(frag)
		switch {
		case s.tooLong:
		case n > s.max || (err != nil && n >= s.max):
			// Over the bound: max-1 bytes and a newline is the longest
			// line, and a final line with no newline gets the same max-1.
			s.tooLong, s.buf = true, s.buf[:0]
		case err == nil && len(s.buf) == 0:
			s.line = frag // whole in r's buffer: no copy
		default:
			s.buf = append(s.grow(len(frag)), frag...)
			s.line = s.buf
		}
		switch {
		case err == nil:
			return s.emit()
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			s.done = true
			if n == 0 {
				return false
			}
			return s.emit()
		default:
			s.done, s.err, s.line, s.tooLong = true, err, nil, false
			return false
		}
	}
}

// grow makes room for n more bytes in buf, doubling but never past max, so
// a line near the bound does not allocate twice the bound.
func (s *Scanner) grow(n int) []byte {
	need := len(s.buf) + n
	if need <= cap(s.buf) {
		return s.buf
	}
	c := min(max(2*cap(s.buf), need), s.max)
	b := make([]byte, len(s.buf), c)
	copy(b, s.buf)
	return b
}

func (s *Scanner) emit() bool {
	if s.tooLong {
		s.skipped++
		s.line = nil
		return true
	}
	l := s.line
	if len(l) > 0 && l[len(l)-1] == '\n' {
		l = l[:len(l)-1]
	}
	if len(l) > 0 && l[len(l)-1] == '\r' {
		l = l[:len(l)-1]
	}
	s.line = l
	return true
}

// Bytes is the current line without its newline, and without a \r before
// it. It is valid until the next Scan. A line over the bound is empty.
func (s *Scanner) Bytes() []byte { return s.line }

// TooLong reports whether the current line was over the bound and stepped
// over.
func (s *Scanner) TooLong() bool { return s.tooLong }

// Skipped is how many lines so far were over the bound.
func (s *Scanner) Skipped() int { return s.skipped }

// Err is the read error that ended the scan, or nil at the end of the input.
func (s *Scanner) Err() error { return s.err }
