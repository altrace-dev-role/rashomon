// Package graphtest is the specimen fixture the contract tests run on: a
// Python monorepo slice (services/auth, services/orgs, services/api, jobs,
// tests, reports) at commit C1, indexed by pylite into an in-memory store.
// It is test support, imported by _test files only.
package graphtest

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

//go:embed specimen
var specimen embed.FS

// Commit is the specimen's committed revision.
const Commit = "C1"

// ErrEditAmbiguous is Edit's refusal when old is absent or not unique.
var ErrEditAmbiguous = errors.New("graphtest: old text absent or not unique")

// Resolver is the specimen's pylite resolver: six units, their module
// roots, and the top-level module map that follows imports across them.
func Resolver() pylite.Resolver {
	return pylite.Resolver{
		Units: []pylite.Unit{
			{Path: "services/auth", ModRoot: "services/auth"},
			{Path: "services/orgs", ModRoot: "services/orgs"},
			{Path: "services/api", ModRoot: "services/api"},
			{Path: "jobs"}, {Path: "tests"}, {Path: "reports"},
		},
		Modules: map[string]string{"auth": "services/auth", "orgs": "services/orgs", "api": "services/api",
			"jobs": "jobs", "tests": "tests", "reports": "reports"},
	}
}

// Specimen is the fixture at C1: the store with every file indexed, the
// files' contents by path (the "disk" an edit is applied to), and the
// checkout root the files were written under.
type Specimen struct {
	Store *graph.Store
	Files map[string][]byte
	Root  string
	Res   pylite.Resolver
}

// New writes the specimen to a temp dir, indexes it at C1 and returns it.
func New(t *testing.T) *Specimen {
	t.Helper()
	root := t.TempDir()
	files := map[string][]byte{}
	err := fs.WalkDir(specimen, "specimen", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := specimen.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "specimen/")
		files[rel] = b
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	res := Resolver()
	recs, err := res.Extract(root, rels, graph.RevCommit(Commit))
	if err != nil {
		t.Fatal(err)
	}
	s := graph.NewStore()
	for _, r := range recs {
		if r.Node != nil {
			s.AssertNode(*r.Node)
		}
		if r.Edge != nil {
			s.AssertEdge(*r.Edge)
		}
	}
	return &Specimen{Store: s, Files: files, Root: root, Res: res}
}

// Edit is the Edit tool's replacement over rel's current content: old must
// occur exactly once. It returns the post-edit content without writing it.
func (sp *Specimen) Edit(rel, old, new string) ([]byte, error) {
	cur := string(sp.Files[rel])
	if strings.Count(cur, old) != 1 {
		return nil, ErrEditAmbiguous
	}
	return []byte(strings.Replace(cur, old, new, 1)), nil
}

// Overlay applies an edit at seq: the post-edit content is parsed by the
// reduced extractor and written as an overlay at {C1, seq}; on success the
// content becomes rel's current content, as the tool would write it. A
// parse failure writes nothing and returns the error.
func (sp *Specimen) Overlay(rel, old, new string, seq int64) (graph.Overlay, error) {
	content, err := sp.Edit(rel, old, new)
	if err != nil {
		return graph.Overlay{}, err
	}
	recs, err := sp.Res.Overlay(rel, content, graph.RevCommit(Commit), seq)
	if err != nil {
		return graph.Overlay{}, err
	}
	ov := sp.Store.ApplyOverlay(graph.FileID(rel), content, recs, graph.RevOverlay(Commit, seq))
	sp.Files[rel] = content
	return ov, nil
}

// Sym is the specimen's func symbol id for a dotted name in a unit.
func Sym(unit, qualified string) graph.ID { return graph.SymID(pylite.Lang, unit, "func", qualified) }

// Const is the specimen's const symbol id.
func Const(unit, qualified string) graph.ID {
	return graph.SymID(pylite.Lang, unit, "const", qualified)
}
