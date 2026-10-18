package store

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// Fold streams path from offset to the last complete line the file holds
// right now, calling fn once per complete line, in file order, and returns
// the byte offset just past the last line folded plus the file's size as
// Fold saw it.
//
// It is ReadRunConsistent's streaming twin (read.go's doc on that function
// explains why a digest needs a read like this at all): where
// ReadRunConsistent loads every line of a file into a *Run and keeps it,
// Fold hands each line to fn and forgets it, so a caller that folds a line
// into compact state rather than a slice never holds more than one line in
// memory, however long the file is. internal/fold's package doc and
// technical spec 8.2.3 step 3 are Fold's one caller's contract; this
// function itself knows nothing about records, prompts or a digest.
//
// A missing file folds nothing, from offset 0 -- the same outcome eachLine's
// other callers already give a run with no spill or no coverage yet.
//
// A final line with no trailing newline is not folded: an append still in
// flight (REC-32) leaves a half-written line at the end of the file, and
// Fold exists to be callable again before that write finishes, so the
// returned offset stops before it rather than counting it skipped the way
// eachLine's forEachLine does. The next Fold call, once the writer completes
// the line, reads it whole. The same rule applies to a line that would be
// reported as over maxLine (lines.go): it is only reported to fn, as nil,
// once its own terminating newline has actually been seen, never while a
// line that happens to already exceed maxLine is still arriving.
//
// offset must be 0 or a value a previous Fold call on the same file
// returned: Fold trusts that offset-1, when offset > 0, is a line
// terminator, and does not re-derive a line boundary by scanning backward.
// A caller holding a saved cursor across a process boundary -- the one this
// exists for -- is responsible for deciding the cursor is still trustworthy
// before calling Fold at all (internal/fold.Validate); Fold's own and only
// defence is the size check below, which catches a file a forget or an
// eviction rewrote shorter than the cursor it was read with (Cursor's
// SizeSeen doc).
//
// Memory is bounded by one line's length (at most maxLine, lines.go),
// regardless of how many bytes lie between offset and the file's end: the
// read itself goes through a bufio.Reader of readChunk, the same buffer
// size forEachLine already uses for every other NDJSON read in this
// package.
func Fold(path string, offset int64, fn func(line []byte)) (newOffset, size int64, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return offset, 0, nil
	}
	if err != nil {
		return offset, 0, err
	}
	defer f.Close() //nolint:errcheck // read-only

	info, err := f.Stat()
	if err != nil {
		return offset, 0, err
	}
	size = info.Size()
	if offset < 0 || offset > size {
		return offset, size, fmt.Errorf("store: fold offset %d is past %s's size %d", offset, path, size)
	}
	if offset == size {
		return offset, size, nil
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return offset, size, err
		}
	}

	folded, err := foldLines(f, offset, fn)
	return folded, size, err
}

// foldLines is Fold's scan, reading from f's current position (already
// seeked to offset) and returning the offset just past the last complete
// line it folded. It is its own function, rather than inlined into Fold,
// because its loop tracks a running byte position that Fold's early returns
// above never need to.
func foldLines(f *os.File, offset int64, fn func(line []byte)) (int64, error) {
	br := bufio.NewReaderSize(f, readChunk)
	var line []byte
	over := false
	pos := offset
	folded := offset
	for {
		chunk, rerr := br.ReadSlice('\n')
		if rerr != nil && !errors.Is(rerr, bufio.ErrBufferFull) && !errors.Is(rerr, io.EOF) {
			return folded, rerr
		}
		terminated := rerr == nil
		pos += int64(len(chunk))
		if terminated {
			chunk = chunk[:len(chunk)-1]
		}
		if !over {
			if len(line)+len(chunk) > maxLine {
				over, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		if !terminated {
			if errors.Is(rerr, io.EOF) {
				// A torn tail: the file ended before this line did. Leave
				// it for the next Fold call rather than folding a line
				// that may still be half-written.
				return folded, nil
			}
			continue // bufio.ErrBufferFull: the rest is in the next chunk
		}
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		switch {
		case over:
			fn(nil)
		default:
			fn(line)
		}
		line, over = nil, false
		folded = pos
	}
}
