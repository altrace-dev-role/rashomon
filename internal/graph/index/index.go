// Package index builds a repository's graph: each declared language's
// extractor over the files under its units, the repository node, and the
// knowledge records and service map from .rashomon/graph.json, and the
// imported rows under .rashomon/knowledge/.
package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// ConfigFile is the repository's graph configuration.
const ConfigFile = ".rashomon/graph.json"

// Config is what a repository declares: its units (one per deployable,
// package or module root, each in a language), the module names each unit
// owns, the service map, and its knowledge records.
type Config struct {
	Units      []Unit            `json:"units"`
	ServiceMap map[string]string `json:"service_map"`
	Knowledge  []Knowledge       `json:"knowledge"`
}

// Unit is a package root: its path, the language whose resolver reads the
// files under it (gopg.Lang, pylite.Lang), the directory its modules start
// at, and the top-level module names it owns. A Go unit is a module root
// to read the pgraph sync for; the units the graph holds for Go are the
// package directories the sync names (gopg).
type Unit struct {
	Path    string   `json:"path"`
	ModRoot string   `json:"mod_root"`
	Modules []string `json:"modules"`
	Lang    string   `json:"lang"`
}

// Knowledge is one lesson or decision: its id (les:... or dec:...), the
// node ids it applies to, and its words.
type Knowledge struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	AppliesTo []string `json:"applies_to"`
	Text      string   `json:"text"`
	// Clause is the one sentence the brief carries for the record, under
	// the approved-text rules (slot.Approved): ASCII, 160 bytes, no
	// double quote; Text is the record's full words, kept for the person.
	Clause string `json:"clause"`
	Source string `json:"source"`
}

// MaxConfigBytes bounds ReadConfig's read of a repository's configuration.
// Nothing in rashomon writes the file; a person does, and nothing but this
// cap bounds its size: it is a bound on a file written by hand, not a size
// measured from one. Over it, `rashomon graph index` fails with an error
// naming the file and the cap, and the Stop names no unit.
const MaxConfigBytes = 64 << 10

// ReadConfig reads root's configuration, for `rashomon graph index` and for
// the Stop (the unit a write is in) alike, so the two cannot disagree on
// whether it reads. It opens through regfile.Open, so a named pipe at the
// path cannot hold the hook (STORE-READ-REGULAR1), and reads at most
// MaxConfigBytes, a file over that being an error that names the file and
// the cap. A missing file is the open's own error, matching fs.ErrNotExist.
func ReadConfig(root string) (Config, error) {
	var c Config
	p := filepath.Join(root, filepath.FromSlash(ConfigFile))
	f, err := regfile.Open(p)
	if err != nil {
		return c, err
	}
	defer f.Close() //nolint:errcheck // read-only
	b, err := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if err == nil && len(b) > MaxConfigBytes {
		err = fmt.Errorf("%s: over %d bytes", p, MaxConfigBytes)
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

// Result is what Build made: the store, the header that names it, and the
// number of files the resolvers were handed.
type Result struct {
	Store  *graph.Store
	Header graph.Header
	Files  int
	// Refused is every file a resolver was handed and refused, by path;
	// the header names the first graph.MaxRefusedNamed.
	Refused []graph.Refusal
}

// Build indexes root at commit: the repository node (baseline.RepoIDErr),
// a declared node per unit, then for each language the configuration
// declares a unit of, that language's resolver over the files under its
// units (a unit the resolver knows better takes the resolver's record),
// then the knowledge records. A file outside every unit of its language,
// and every file of an undeclared language, is not indexed. A file a
// resolver refuses (graph.Refusals: it could not be read, or its extractor
// cannot parse it) is a gap, not a failure: the index is written without
// it, Result.Refused lists every such file by path, and the header counts
// them and names the first graph.MaxRefusedNamed, so a reader of the index
// knows an absence in it is not proven. The header names the repository,
// the commit, and each producer that wrote records. Any other resolver
// error is the error. A refused git read (a pointer, commondir or config that is not a regular
// file or is over baseline.MaxGitFileBytes) is the error: no index is keyed
// under no id, or under the path form a later build would not give.
func Build(root, commit string, c Config) (Result, error) {
	ps, err := producers(c)
	if err != nil {
		return Result{}, err
	}
	files, total, err := claimed(root, ps)
	if err != nil {
		return Result{}, err
	}
	rev := graph.RevCommit(commit)
	s := graph.NewStore()
	rid, ephemeral, err := baseline.RepoIDErr(root)
	if err != nil {
		return Result{}, err
	}
	s.AssertNode(repoNode(rid, ephemeral, rev))
	for _, u := range c.Units {
		s.AssertNode(graph.Node{ID: graph.UnitID(u.Lang, u.Path), Kind: "unit", Tier: graph.Declared, Conf: 1.0, Rev: rev,
			Attrs: map[string]string{"path": u.Path, "lang": u.Lang}})
	}
	wrote := map[string]string{}
	var refused []graph.Refusal
	for i, p := range ps {
		recs, err := p.res.Extract(root, files[i], rev)
		var rf graph.Refusals
		switch {
		case errors.As(err, &rf):
			refused = append(refused, rf...)
		case err != nil:
			return Result{}, err
		}
		if v, ok := producerOf(recs); ok {
			wrote[p.res.Lang()] = v
		}
		apply(s, recs)
	}
	apply(s, declared(c, rev))
	rows, err := knowledge.ReadDir(root)
	if err != nil {
		return Result{}, err
	}
	apply(s, knowledge.Records(rows, rid, rev))
	sort.SliceStable(refused, func(a, b int) bool { return refused[a].Path < refused[b].Path })
	h := graph.Header{Schema: graph.Schema, Repo: rid, Rev: graph.HeaderRev{Commit: commit}, Producers: wrote,
		Refused: len(refused), RefusedFiles: slices.Clone(refused[:min(len(refused), graph.MaxRefusedNamed)])}
	return Result{Store: s, Header: h, Files: total, Refused: refused}, nil
}

// producer is one declared language's resolver and its units' paths.
type producer struct {
	res   lang.Resolver
	paths []string
}

// producers is the resolver per language the configuration declares, in
// order of first appearance; a unit with no lang, or a lang no resolver is
// wired for, is an error naming it.
func producers(c Config) ([]producer, error) {
	var out []producer
	at := map[string]int{}
	for _, u := range c.Units {
		if u.Lang == "" {
			return nil, fmt.Errorf("graph: unit %q declares no lang", u.Path)
		}
		i, seen := at[u.Lang]
		if !seen {
			res, err := resolverFor(u.Lang, c)
			if err != nil {
				return nil, err
			}
			i = len(out)
			at[u.Lang] = i
			out = append(out, producer{res: res})
		}
		out[i].paths = append(out[i].paths, u.Path)
	}
	return out, nil
}

// resolverFor is the resolver wired for a language: this is the one place
// the index names languages.
func resolverFor(name string, c Config) (lang.Resolver, error) {
	switch name {
	case gopg.Lang:
		return gopg.Resolver{}, nil
	case pylite.Lang:
		return python(c), nil
	}
	return nil, fmt.Errorf("graph: unit lang %q is not %s or %s", name, gopg.Lang, pylite.Lang)
}

// python is the Python resolver the configuration's Python units declare.
func python(c Config) pylite.Resolver {
	r := pylite.Resolver{Modules: map[string]string{}}
	for _, u := range c.Units {
		if u.Lang != pylite.Lang {
			continue
		}
		r.Units = append(r.Units, pylite.Unit{Path: u.Path, ModRoot: u.ModRoot})
		for _, m := range u.Modules {
			r.Modules[m] = u.Path
		}
	}
	return r
}

// claimed lists, per producer, the repository's files its resolver claims
// under its units, as slash paths relative to root, sorted; and the count.
// The walk skips the directories skipDir names, and claims.producer picks a
// file's producer: the one rule UnitOf files a single path by.
func claimed(root string, ps []producer) ([][]string, int, error) {
	cl := claimsOf(ps)
	files, total := make([][]string, len(ps)), 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if i, ok := cl.producer(root, rel); ok {
			files[i] = append(files[i], rel)
			total++
		}
		return nil
	})
	for i := range files {
		sort.Strings(files[i])
	}
	return files, total, err
}

// claims is the rule Build claims a file by: the producers' resolvers in a
// registry, in configuration order, and each language's producer.
type claims struct {
	ps  []producer
	reg lang.Registry
	at  map[string]int
}

func claimsOf(ps []producer) claims {
	c := claims{ps: ps, at: map[string]int{}}
	for i, p := range ps {
		c.reg.Register(p.res)
		c.at[p.res.Lang()] = i
	}
	return c
}

// producer is the index in ps of the producer whose resolver claims rel, a
// slash path relative to root, false where no resolver claims it or rel is
// under none of that producer's units.
func (c *claims) producer(root, rel string) (int, bool) {
	res, ok := c.reg.For(root, rel)
	if !ok {
		return 0, false
	}
	i := c.at[res.Lang()]
	return i, under(rel, c.ps[i].paths)
}

// skipDir reports whether Build's walk skips a directory named name: a dot
// directory, node_modules or __pycache__ (W-DEBT.6).
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__"
}

// UnitOf is the unit an index built under c files a path in, by the rule
// Build claims files by: rel, a slash path relative to the index root, is
// under no directory Build's walk skips, a resolver claims it, and it is
// under one of that resolver's language's units; the resolver's Unit then
// names it. false for a path Build would not index. It reads no file, so
// the Stop maps the paths a session wrote with it without loading the
// graph (W-M3.3); the error is the configuration's, as Build's would be.
func UnitOf(c Config) (func(root, rel string) (graph.ID, bool), error) {
	ps, err := producers(c)
	if err != nil {
		return nil, err
	}
	cl := claimsOf(ps)
	return func(root, rel string) (graph.ID, bool) {
		dirs := strings.Split(rel, "/")
		for _, d := range dirs[:len(dirs)-1] {
			if skipDir(d) {
				return "", false
			}
		}
		i, ok := cl.producer(root, rel)
		if !ok {
			return "", false
		}
		return cl.ps[i].res.Unit(root, rel)
	}, nil
}

// under reports whether rel is at or below one of paths ("." is the root).
func under(rel string, paths []string) bool {
	for _, p := range paths {
		if p == "." || p == "" || rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

// producerOf is extractor/version from the first record with provenance.
func producerOf(recs []graph.Record) (string, bool) {
	for _, r := range recs {
		var p graph.Prov
		switch {
		case r.Node != nil:
			p = r.Node.Prov
		case r.Edge != nil:
			p = r.Edge.Prov
		}
		if p.Extractor != "" {
			return p.Extractor + "/" + p.Version, true
		}
	}
	return "", false
}

// repoNode is the node that stands for the repository: its id as the
// header carries it, and whether that id is ephemeral.
func repoNode(rid string, ephemeral bool, rev graph.Rev) graph.Node {
	attrs := map[string]string{"rid": rid}
	if ephemeral {
		attrs["ephemeral"] = "true"
	}
	return graph.Node{ID: graph.RepoNodeID(rid), Kind: "repo", Tier: graph.Proven, Conf: 1.0, Rev: rev, Attrs: attrs,
		Prov: graph.Prov{Extractor: "index", Version: "1"}}
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
func declared(c Config, rev graph.Rev) []graph.Record {
	var recs []graph.Record
	for _, k := range c.Knowledge {
		if k.ID == "" || k.Kind == "" {
			continue
		}
		id := graph.ID(k.ID)
		recs = append(recs, graph.Record{Node: &graph.Node{ID: id, Kind: k.Kind, Tier: graph.Declared, Conf: 1.0, Rev: rev,
			Attrs: map[string]string{"title": k.Title, "text": k.Text, "clause": k.Clause, "source": k.Source}}})
		for _, t := range k.AppliesTo {
			recs = append(recs, graph.Record{Edge: &graph.Edge{Src: id, Pred: graph.AppliesTo, Dst: graph.ID(t), Tier: graph.Declared, Conf: 1.0, Rev: rev}})
		}
	}
	return recs
}

// ErrNoConfig is Build's answer when root has no configuration.
var ErrNoConfig = errors.New("graph: no " + ConfigFile)
