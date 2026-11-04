// Package index builds a repository's graph: the language extractors over
// its files, the knowledge records and service map from .rashomon/graph.json.
package index

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// ConfigFile is the repository's graph configuration.
const ConfigFile = ".rashomon/graph.json"

// Config is what a repository declares: its units (one per deployable or
// package root), the module names each unit owns, the service map, and
// its knowledge records.
type Config struct {
	Units      []Unit            `json:"units"`
	ServiceMap map[string]string `json:"service_map"`
	Knowledge  []Knowledge       `json:"knowledge"`
}

// Unit is a package root: its path, the directory its modules start at,
// and the top-level module names it owns.
type Unit struct {
	Path    string   `json:"path"`
	ModRoot string   `json:"mod_root"`
	Modules []string `json:"modules"`
}

// Knowledge is one lesson or decision: its id (les:... or dec:...), the
// node ids it applies to, and its words.
type Knowledge struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	AppliesTo []string `json:"applies_to"`
	Text      string   `json:"text"`
	Source    string   `json:"source"`
}

// ReadConfig reads root's configuration.
func ReadConfig(root string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ConfigFile)))
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

// Resolver builds the Python resolver the configuration declares.
func Resolver(c Config) pylite.Resolver {
	r := pylite.Resolver{Modules: map[string]string{}}
	for _, u := range c.Units {
		r.Units = append(r.Units, pylite.Unit{Path: u.Path, ModRoot: u.ModRoot})
		for _, m := range u.Modules {
			r.Modules[m] = u.Path
		}
	}
	return r
}

// Build indexes root at commit: every file a resolver claims, then the
// knowledge records.
func Build(root, commit string, c Config) (*graph.Store, int, error) {
	res := Resolver(c)
	reg := lang.Registry{}
	reg.Register(res)
	files, err := claimed(root, reg)
	if err != nil {
		return nil, 0, err
	}
	rev := graph.RevCommit(commit)
	s := graph.NewStore()
	for _, u := range c.Units {
		s.AssertNode(graph.Node{ID: graph.UnitID(res.Lang(), u.Path), Kind: "unit", Tier: graph.Declared, Conf: 1.0, Rev: rev,
			Attrs: map[string]string{"path": u.Path}})
	}
	recs, err := res.Extract(root, files, rev)
	if err != nil {
		return nil, 0, err
	}
	apply(s, recs)
	apply(s, knowledge(c, rev))
	return s, len(files), nil
}

// claimed lists the repository's files a registered resolver claims, as
// slash paths relative to root, sorted.
func claimed(root string, reg lang.Registry) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "__pycache__") {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := reg.For(root, rel); ok {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func apply(s *graph.Store, recs []graph.Record) {
	for _, r := range recs {
		switch {
		case r.Node != nil:
			s.AssertNode(*r.Node)
		case r.Edge != nil:
			s.AssertEdge(*r.Edge)
		}
	}
}

// knowledge is the records of the configuration's lessons and decisions:
// a declared node each, and an applies_to edge to every target.
func knowledge(c Config, rev graph.Rev) []graph.Record {
	var recs []graph.Record
	for _, k := range c.Knowledge {
		if k.ID == "" || k.Kind == "" {
			continue
		}
		id := graph.ID(k.ID)
		recs = append(recs, graph.Record{Node: &graph.Node{ID: id, Kind: k.Kind, Tier: graph.Declared, Conf: 1.0, Rev: rev,
			Attrs: map[string]string{"title": k.Title, "text": k.Text, "source": k.Source}}})
		for _, t := range k.AppliesTo {
			recs = append(recs, graph.Record{Edge: &graph.Edge{Src: id, Pred: graph.AppliesTo, Dst: graph.ID(t), Tier: graph.Declared, Conf: 1.0, Rev: rev}})
		}
	}
	return recs
}

// ErrNoConfig is Build's answer when root has no configuration.
var ErrNoConfig = errors.New("graph: no " + ConfigFile)
