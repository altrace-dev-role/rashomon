package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Error is a problem found in a settings file, with where it was found.
//
// It renders as path:line:column: problem, the form editors and compilers
// use, so a person told their settings.json will not parse can go to the
// place. Line and Column are 1-based and count bytes, as Go's own tools do,
// and point at the byte the parser had just read when it stopped. Both are
// zero when the problem has no place in the file: a value this package
// rewrote, whose bytes now sit at offsets the file does not hold. A position
// is never a guess; a missing one is better than a wrong one.
//
// Path is empty for a document parsed from bytes with no name (Parse).
type Error struct {
	Path   string
	Line   int
	Column int
	Err    error
}

func (e *Error) Error() string {
	where := e.Path
	if where == "" {
		where = "settings"
	}
	if e.Line > 0 {
		where = fmt.Sprintf("%s:%d:%d", where, e.Line, e.Column)
	}
	return where + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// pos is a byte offset into the parsed source plus one, so that the zero value
// means "nowhere": a member built by this package, or a value rewritten after
// parsing, can never claim offset 0 by default.
type pos int

func posAt(offset int) pos { return pos(offset + 1) }

func (p pos) offset() (int, bool) { return int(p) - 1, p > 0 }

// in moves a position found inside a value to the source the value came from,
// given where that value starts. Either one being nowhere leaves nowhere.
func (p pos) in(base pos) pos {
	if p == 0 || base == 0 {
		return 0
	}
	return p + base - 1
}

// offsetError carries the position inside the bytes parseObject was given.
// Its text is the error's own, so wrapping it changes no message.
type offsetError struct {
	at  pos
	err error
}

func (e *offsetError) Error() string { return e.err.Error() }
func (e *offsetError) Unwrap() error { return e.err }

// refusal locates a parseObject failure in data, the bytes it was parsing.
//
// A JSON syntax error is placed by the scanner json.Unmarshal runs over the
// whole of data: it reports the first byte that cannot be JSON, where the
// decoder's token-level errors report the end of the last good token (a
// trailing comma reads as the comma, not the brace after it). That scan runs
// only here, on the error path, so a healthy parse pays nothing for it. This
// package's own refusals of valid JSON -- a duplicate key, a value that is not
// an object -- are placed at the last byte of the token at fault, which is
// where the decoder stands. Trailing data after a complete object (closed) is
// a syntax error to the scanner, which places it at its first byte.
func refusal(data []byte, decoded int64, closed bool, err error) error {
	at := posAt(max(int(decoded)-1, 0))
	var syn *json.SyntaxError
	syntactic := errors.As(err, &syn) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
	if syntactic || closed {
		if errors.As(json.Unmarshal(data, new(json.RawMessage)), &syn) {
			at = posAt(max(int(syn.Offset)-1, 0))
			if syntactic {
				// The scanner's text names the byte it placed; a bare EOF
				// from the decoder names nothing.
				err = syn
			}
		}
	}
	return &offsetError{at: at, err: err}
}

// fail builds the *Error for err, found in a value that starts at base. An
// offsetError inside err is the more precise place and wins.
func (d *Document) fail(base pos, err error) error {
	at := base
	var oe *offsetError
	if errors.As(err, &oe) {
		at = oe.at.in(base)
	}
	e := &Error{Path: d.path, Err: err}
	if off, ok := at.offset(); ok && off <= len(d.src) {
		e.Line, e.Column = lineColumn(d.src, off)
	}
	return e
}

// lineColumn is the 1-based line and byte column of offset off in src.
func lineColumn(src []byte, off int) (line, column int) {
	before := src[:off]
	line = bytes.Count(before, []byte{'\n'}) + 1
	column = off - (bytes.LastIndexByte(before, '\n') + 1) + 1
	return line, column
}
