package store

import (
	"bufio"
	"errors"
	"io"
)

// maxLine bounds a single line on read. A record is far smaller by
// construction (MaxRecordBytes, limits.go); the bound is what stops a damaged
// file from being read as one giant line. It is unchanged so that lines
// written before the writer was bounded still read.
const maxLine = 16 << 20

// readChunk is the reader's buffer. A line that fits is copied once; a longer
// one is assembled, up to maxLine, from chunks of this size, so memory grows
// with the longest line actually read and never past maxLine.
const readChunk = 64 << 10

// errLineTooLong is a file that holds a line longer than maxLine. Readers
// count such a line as unreadable and go on; only a rewrite, which would have
// to write the line back and cannot hold it, stops at it.
var errLineTooLong = errors.New("store: a line is longer than the reader's limit")

// forEachLine calls fn for every non-empty line of r, without its line
// ending, in a slice fn may keep.
//
// A line longer than maxLine is not read. It is discarded up to its newline,
// and fn is called with a nil line in its place: nil parses as nothing, so
// every caller counts it exactly as it counts a damaged line, and the lines
// after it still read. bufio.Scanner, which this replaces, failed the whole
// read on such a line, so one record over the limit hid every record of its
// run from report, from the end probe, from forget and from eviction (lld-3).
//
// A last line with no newline -- a torn append -- is handed over like any
// other, and parses or not on its merits.
func forEachLine(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, readChunk)
	var line []byte
	over := false
	for {
		chunk, err := br.ReadSlice('\n')
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) && !errors.Is(err, io.EOF) {
			return err
		}
		ended := err == nil
		if ended {
			chunk = chunk[:len(chunk)-1]
		}
		if !over {
			if len(line)+len(chunk) > maxLine {
				over, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		if !ended && !errors.Is(err, io.EOF) {
			continue // the rest of this line is in the next chunk
		}
		// bufio.Scanner dropped one carriage return before the newline, and
		// the readers keep that behaviour; a record never holds a raw one.
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		switch {
		case over:
			fn(nil)
		case len(line) > 0:
			fn(line)
		}
		line, over = nil, false
		if !ended {
			return nil
		}
	}
}
