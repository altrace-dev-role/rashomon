package graph

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
}

// HeaderRev is the committed revision a file's records are at.
type HeaderRev struct {
	Commit string `json:"commit"`
}

// ErrNoGraph is Load's answer when the file is absent.
var ErrNoGraph = errors.New("graph: no index")

// GraphFile is where a repository's index lives, under its root.
const GraphFile = ".rashomon/graph.jsonl"

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

// SaveFile writes the index under root, under h (Save).
func SaveFile(root string, s *Store, h Header) error {
	path := filepath.Join(root, filepath.FromSlash(GraphFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "graph-*.tmp")
	if err != nil {
		return err
	}
	if err := Save(f, s, h); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), path)
}

// LoadFile reads the index under root; ErrNoGraph when there is none.
func LoadFile(root string) (*Store, Header, error) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(GraphFile)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Header{}, ErrNoGraph
		}
		return nil, Header{}, err
	}
	defer f.Close()
	return Load(f)
}

// MaxIndexDepth is how many parent directories FindIndex climbs: a
// repository's index may sit in a workspace directory holding several
// repositories, so the graph can span them.
const MaxIndexDepth = 4

// FindIndex is the nearest directory at or above dir holding an index, or
// "" when none does within MaxIndexDepth.
func FindIndex(dir string) string {
	for i := 0; i <= MaxIndexDepth && dir != ""; i++ {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(GraphFile))); err == nil {
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
