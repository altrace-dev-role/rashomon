package graph

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Snapshot is a store's committed records on disk: a header line naming
// the commit, then one Record per line (JSONL), nodes before edges.
// Overlays are session state and are not written.
type Header struct {
	Commit  string
	Records int
}

// ErrNoGraph is Load's answer when the file is absent.
var ErrNoGraph = errors.New("graph: no index")

// GraphFile is where a repository's index lives, under its root.
const GraphFile = ".rashomon/graph.jsonl"

// Save writes the store's records at commit rev to w.
func Save(w io.Writer, s *Store, commit string) error {
	rev := RevCommit(commit)
	nodes, edges := s.Nodes(rev), s.Edges(rev)
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	if err := enc.Encode(Header{Commit: commit, Records: len(nodes) + len(edges)}); err != nil {
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

// Load reads a store from r: the header, then every record.
func Load(r io.Reader) (*Store, Header, error) {
	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<16))
	var h Header
	if err := dec.Decode(&h); err != nil {
		return nil, h, err
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

// SaveFile writes the index under root.
func SaveFile(root string, s *Store, commit string) error {
	path := filepath.Join(root, filepath.FromSlash(GraphFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "graph-*.tmp")
	if err != nil {
		return err
	}
	if err := Save(f, s, commit); err != nil {
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
