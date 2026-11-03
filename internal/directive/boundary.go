package directive

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// Boundary is a compaction's mark in a transcript: the uuid of its
// compact_boundary record and the number of that record's line, counted
// from 1.
type Boundary struct {
	UUID string
	Line int
}

// ErrTooLarge is LastBoundary's refusal of a transcript over its read cap.
var ErrTooLarge = errors.New("directive: transcript over the read cap")

// maxBoundaryLine is the longest line read for a boundary. A compact_boundary
// record is a short system line, so a longer line is passed over unread.
const maxBoundaryLine = 64 << 10

// LastBoundary is the transcript's last compact_boundary record, read forward
// once with every line counted, so its Line is the number Read's directives
// are numbered in. A transcript over capBytes is not read (ErrTooLarge), and
// false says it holds none.
func LastBoundary(path string, capBytes int64) (Boundary, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return Boundary{}, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Boundary{}, false, err
	}
	if fi.Size() > capBytes {
		return Boundary{}, false, ErrTooLarge
	}
	r := bufio.NewReaderSize(f, maxBoundaryLine)
	var last Boundary
	found := false
	for n := 1; ; n++ {
		line, long, err := readLine(r)
		if !long {
			if id, ok := boundaryUUID(line); ok {
				last, found = Boundary{UUID: id, Line: n}, true
			}
		}
		if err == io.EOF {
			return last, found, nil
		}
		if err != nil {
			return Boundary{}, false, err
		}
	}
}

// readLine is the reader's next line without its line feed, valid until the
// next read, and long when the line ran past the reader's buffer, in which
// case the rest of it is read past and not returned.
func readLine(r *bufio.Reader) (line []byte, long bool, err error) {
	line, err = r.ReadSlice('\n')
	for err == bufio.ErrBufferFull {
		long = true
		_, err = r.ReadSlice('\n')
	}
	return bytes.TrimSuffix(line, []byte{'\n'}), long, err
}

// The top-level keys a boundary is read by, in boundaryKeys' sorted order.
const (
	bkSubtype = iota
	bkType
	bkUUID
	bkCount
)

// boundaryKeys is the one KeySet a transcript line is read with (RES-5: no
// decode of a whole line).
func boundaryKeys() *jsonscan.KeySet {
	boundaryKeysOnce.Do(func() { boundaryKeysOnce.v = jsonscan.NewKeySet("subtype", "type", "uuid") })
	return boundaryKeysOnce.v
}

// boundaryKeysOnce holds what boundaryKeys returns, built on its first call
// and never at init (RES-2).
var boundaryKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

// boundaryUUID is the uuid of a compact_boundary record, a system line whose
// subtype is compact_boundary, and false for any other line. Lines without
// the word are passed over before any parse.
func boundaryUUID(line []byte) (string, bool) {
	if !bytes.Contains(line, []byte(`"compact_boundary"`)) {
		return "", false
	}
	var out [bkCount]jsonscan.Span
	if jsonscan.Top(line, boundaryKeys(), out[:]) != nil || text(line, out[bkType]) != "system" ||
		text(line, out[bkSubtype]) != "compact_boundary" {
		return "", false
	}
	id := text(line, out[bkUUID])
	return id, id != ""
}

// text is a string value's text, "" for any other kind or a bad escape.
func text(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}
