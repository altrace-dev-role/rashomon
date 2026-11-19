package graph

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
	"github.com/altrace-dev-role/rashomon/internal/rootio"
)

// Schema is the store file's format this package writes and reads.
const Schema = 1

// Header is the store file's first line (PLAN B6); one Record per line
// follows (JSONL), nodes before edges, overlays never (they are session
// state). It names the schema, the repository (baseline.RepoID: the
// normalized origin url, else an ephemeral path form), the commit the
// records are at, each producer that wrote records as extractor/version by
// language, and the record count. Records hold local ids; the id a row
// outside this file carries is GlobalID(Repo, local).
type Header struct {
	Schema    int               `json:"schema"`
	Repo      string            `json:"repo"`
	Rev       HeaderRev         `json:"rev"`
	Producers map[string]string `json:"producers"`
	Records   int               `json:"records"`
	// Refused is how many files a producer was handed and refused, and
	// RefusedFiles the first MaxRefusedNamed of them by path: the index
	// holds none of their records, so a name one of them declares reads
	// as absent though it is not. Both are left out when nothing was
	// refused, so such a header is byte-equal to one written before them;
	// the writers before them failed the whole index on a refusal
	// (pylite's Extract, pgraph's load), so a header without them reads
	// as no file refused.
	Refused      int       `json:"refused,omitempty"`
	RefusedFiles []Refusal `json:"refused_files,omitempty"`
}

// MaxRefusedNamed is how many refused files a header names, so its line
// stays bounded for a reader that reads the header alone.
const MaxRefusedNamed = 20

// Refusal is one file a resolver was handed and left out of the index:
// its slash path relative to the index root, the line its reader stopped
// at (0 when the refusal has none, as a read error has none), and why, in
// the resolver's words.
type Refusal struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Reason string `json:"reason"`
}

// Refusals is the error a resolver's Extract returns, beside the records
// of every file it read, when it refused some: the index is written
// without them and names each one. Any other error fails the index.
type Refusals []Refusal

func (r Refusals) Error() string {
	return fmt.Sprintf("%d file(s) refused", len(r))
}

// HeaderRev is the committed revision a file's records are at.
type HeaderRev struct {
	Commit string `json:"commit"`
}

// ErrNoGraph is Load's answer when the file is absent. Its words say where
// the index was looked for and what writes one.
var ErrNoGraph = errors.New("graph: no index at " + GraphFile + "; run rashomon graph index at the repository root")

// GraphFile is where a repository's index lives, under its root.
const GraphFile = ".rashomon/graph.jsonl"

// MaxGraphFileBytes caps GraphFile: 64 MiB. Its one home is here. LoadFile
// refuses a file over it, pinned by its test; the writer is to hold the
// same cap by this name (card WRITE-SAFE1), so that what is written can be
// read. Every prompt decodes the whole index, so the cap bounds the memory
// and the time of a hook. The cap does not grow with the repository: an
// index over it is refused until a read path exists that does not decode
// the whole file. Not verify.MaxIndexBytes, which caps another file, the
// project index of verify.
const MaxGraphFileBytes = 64 << 20

// Save writes the store's records at h's commit to w under h, with the
// schema and the record count filled here. A header naming no repository
// or no commit is refused: every reader must know both.
func Save(w io.Writer, s *Store, h Header) error {
	if h.Repo == "" || h.Rev.Commit == "" {
		return errors.New("graph: the header needs a repo and a rev commit")
	}
	rev := RevCommit(h.Rev.Commit)
	nodes, edges := s.Nodes(rev), s.Edges(rev)
	h.Schema, h.Records = Schema, len(nodes)+len(edges)
	if h.Producers == nil {
		h.Producers = map[string]string{}
	}
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	if err := enc.Encode(h); err != nil {
		return err
	}
	for i := range nodes {
		if err := enc.Encode(Record{Node: &nodes[i]}); err != nil {
			return err
		}
	}
	for i := range edges {
		if err := enc.Encode(Record{Edge: &edges[i]}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// Load reads a store from r: the header, then every record. A file under
// another schema, or one written before the schema line, is an error.
func Load(r io.Reader) (*Store, Header, error) {
	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<16))
	var h Header
	if err := dec.Decode(&h); err != nil {
		return nil, h, err
	}
	if h.Schema != Schema {
		return nil, h, fmt.Errorf("graph: index schema %d, want %d; run rashomon graph index", h.Schema, Schema)
	}
	s := NewStore()
	for {
		var rec Record
		if err := dec.Decode(&rec); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, h, err
		}
		switch {
		case rec.Node != nil:
			s.AssertNode(*rec.Node)
		case rec.Edge != nil:
			s.AssertEdge(*rec.Edge)
		}
	}
	return s, h, nil
}

// SaveFile writes the index under root, under h (Save), through a root
// opened at root, so a link on the index's path cannot carry the write out
// of the repository; a link at the index file itself is replaced. The file
// is created with mode 0600, as the temporary file it replaces was.
func SaveFile(root string, s *Store, h Header) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck // the root holds only a directory handle
	return rootio.ReplaceFile(r, filepath.FromSlash(GraphFile), 0o600, func(w io.Writer) error { return Save(w, s, h) })
}

// LoadFile reads the index under root; ErrNoGraph when there is none. The
// file is opened by regfile.OpenCapped, so a named pipe or a device at the
// name is refused at once and never waited on, and an index over
// MaxGraphFileBytes, by its size at the open or grown past it while it is
// read, is refused with a line that names its size, the cap and the act
// that writes it again (overCap); the error still matches
// regfile.ErrOverCap.
func LoadFile(root string) (*Store, Header, error) {
	rc, err := regfile.OpenCapped(filepath.Join(root, filepath.FromSlash(GraphFile)), MaxGraphFileBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Header{}, ErrNoGraph
		}
		return nil, Header{}, overCap(err)
	}
	defer rc.Close() //nolint:errcheck // read only
	s, h, err := Load(rc)
	return s, h, overCap(err)
}

// overCap is err, or, where err refused the index over its cap, the line a
// person reads for it, wrapping the refusal.
func overCap(err error) error {
	var oc *regfile.OverCapError
	if errors.As(err, &oc) {
		return fmt.Errorf("graph: the index %s is %w; run rashomon graph index to write it again", GraphFile, oc)
	}
	return err
}

// MaxIndexDepth is how many parent directories FindIndex climbs: a
// repository's index may sit in a workspace directory holding several
// repositories, so the graph can span them.
const MaxIndexDepth = 4

// FindIndex is the nearest directory at or above dir holding an index, or
// "" when none does within MaxIndexDepth. Each level is one stat of the
// index's path (indexAt), which on unix allocates only that path and its
// NUL-terminated copy: a hook call on a Go file outside any index pays the
// walk on every edit (the hook's allocation budgets, test/perf/budgets.json).
func FindIndex(dir string) string {
	for i := 0; i <= MaxIndexDepth && dir != ""; i++ {
		if indexAt(filepath.Join(dir, filepath.FromSlash(GraphFile))) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
