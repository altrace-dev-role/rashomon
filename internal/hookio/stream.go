package hookio

import (
	"io"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// IntakeStream walks one payload from r without holding it, passing the
// values at want to fn as jsonscan.Stream does. The payload must be one JSON
// object (or null), as the buffered entries require; no size cap applies,
// because nothing but the wanted captures is kept. It serves recap's Stop and
// catch-up; P-16 moves PostToolBatch, SubagentStop and PostCompact onto it.
func IntakeStream(r io.Reader, want []jsonscan.Path, fn func(path int, value []byte, truncated bool) bool) error {
	paths := make([]jsonscan.Path, len(want)+1)
	copy(paths, want)
	root := len(want)
	paths[root] = jsonscan.Path{Max: 1, Mode: jsonscan.CaptureHead}
	var first byte
	_, err := jsonscan.StreamOne(r, paths, func(i int, v []byte, truncated bool) bool {
		if i == root {
			if len(v) > 0 {
				first = v[0]
			}
			return true
		}
		return fn(i, v, truncated)
	})
	if err != nil {
		return err
	}
	if first != 0 && first != '{' {
		return jsonscan.ErrNotObject
	}
	return nil
}
