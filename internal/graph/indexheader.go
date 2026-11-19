package graph

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// maxHeaderBytes bounds the read of an index's first line: a header names a
// repository, a commit and a few producers.
const maxHeaderBytes = 64 << 10

// ReadIndexHeader is the Header of the index file at path, from its first
// line only: no record is read, and none is loaded. A hook that needs the
// index's commit and nothing else (the overlay writers) uses it so the cost
// is one small read, never the graph. A file of another schema is an error.
// The open is regfile.Open's, so a named pipe at the path is refused at once
// and never waited on (a hook would wait with it).
func ReadIndexHeader(path string) (Header, error) {
	var h Header
	f, err := regfile.Open(path)
	if err != nil {
		return h, err
	}
	defer f.Close() //nolint:errcheck // read only
	if err := json.NewDecoder(io.LimitReader(f, maxHeaderBytes)).Decode(&h); err != nil {
		return Header{}, err
	}
	if h.Schema != Schema {
		return Header{}, fmt.Errorf("graph: index schema %d, want %d; run rashomon graph index", h.Schema, Schema)
	}
	return h, nil
}
