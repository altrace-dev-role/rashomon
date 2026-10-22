package hookio

import (
	"errors"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// ErrStopped reports a walk fn ended early. The payload past that point was
// neither read nor validated, so what fn was passed comes from a payload
// nobody has checked whole.
var ErrStopped = errors.New("hookio: the walk was stopped before the payload ended")

// IntakeStream walks one payload from r without holding it, passing the
// values at want to fn as jsonscan.Stream does. The payload must be one JSON
// object (or null), as decoding into the payload structs required: the first
// byte past leading whitespace decides that, before anything is parsed, so a
// top-level string is refused whatever its text and fn is never called for
// it (jsonscan.ErrNotObject). No size cap applies, because nothing but the
// wanted captures is kept; a caller bounds r. fn returning false stops the
// walk with ErrStopped. It serves recap's Stop and catch-up; P-16 moves
// PostToolBatch, SubagentStop and PostCompact onto it.
func IntakeStream(r io.Reader, want []jsonscan.Path, fn func(path int, value []byte, truncated bool) bool) error {
	stopped := false
	_, err := jsonscan.StreamOne(&objectReader{r: r}, want, func(i int, v []byte, truncated bool) bool {
		stopped = !fn(i, v, truncated)
		return !stopped
	})
	if err == nil && stopped {
		return ErrStopped
	}
	return err
}

// objectReader passes r through and fails the read that shows the first
// value is not an object or null, before any of it reaches the parser.
type objectReader struct {
	r    io.Reader
	seen bool
}

func (o *objectReader) Read(p []byte) (int, error) {
	n, err := o.r.Read(p)
	for _, c := range p[:n] {
		if o.seen {
			break
		}
		switch c {
		case ' ', '\t', '\n', '\r':
		case '{', 'n':
			o.seen = true
		default:
			return 0, jsonscan.ErrNotObject
		}
	}
	return n, err
}
