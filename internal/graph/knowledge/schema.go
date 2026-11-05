// Package knowledge is the judgment store's rows: decisions, lessons and
// the other asserted kinds, imported from a repository's own written
// history into .rashomon/knowledge/<source>.json, keyed on the source's
// own ids and applied to code by global ids (docs/autosteer/PLAN.md Part
// B6, row schema v1). The index reads the directory and asserts a node per
// row with an applies_to edge per target in this repository.
package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

const (
	// Dir holds one file per imported source under the repository root.
	Dir = ".rashomon/knowledge"
	// Schema is the row schema this package writes and reads.
	Schema = 1
)

// The row kinds (Part B6).
const (
	KindDecision   = "decision"
	KindLesson     = "lesson"
	KindIncident   = "incident"
	KindConvention = "convention"
	KindException  = "exception"
	KindFlow       = "flow"
	KindObjective  = "objective"
)

// The lifecycle statuses: a row is a candidate until a person approves it.
const (
	StatusCandidate  = "candidate"
	StatusApproved   = "approved"
	StatusRetired    = "retired"
	StatusSuperseded = "superseded"
)

// The judgment tiers a row carries (AD4): an imported row is INFERRED
// until approved, an approved source is DECLARED, and EARNED comes from
// tallies under labels (M5).
const (
	TierInferred = "INFERRED"
	TierDeclared = "DECLARED"
	TierEarned   = "EARNED"
)

// Source is where a row came from: the file by its base name (a record
// by its directory and name), the row within it, and the file's sha256.
type Source struct {
	File   string `json:"file"`
	Row    string `json:"row"`
	Digest string `json:"digest"`
}

// Scope narrows a row to paths and languages; empty, applies_to decides.
type Scope struct {
	Paths     []string `json:"paths"`
	Languages []string `json:"languages"`
}

// Tally is the row's record under labels: k honored of n delivered.
type Tally struct {
	K int `json:"k"`
	N int `json:"n"`
}

// Step is one step of a flow row (Part B4), present and empty until M3.
type Step struct {
	Kind      string `json:"kind"`
	Ref       string `json:"ref"`
	Direction string `json:"direction"`
}

// Invariant is one invariant of a flow row (Part B4).
type Invariant struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Predicate is one deterministic predicate of an objective row (Part B3).
type Predicate struct {
	Kind string `json:"kind"`
	Arg  string `json:"arg"`
}

// Row is one knowledge row, schema v1. Every lifecycle field is present
// from the first import, so M3 flows, M4 objectives and M5 lifecycle
// write into it without a migration. ID is the source's own id (B227,
// D161, ADR-0007); NodeID keys it in the store. Clause is the one sentence
// a brief may carry, under the approved-text rules, or "" for a row that
// is title only. AppliesTo holds global ids.
type Row struct {
	Schema      int         `json:"schema"`
	ID          string      `json:"id"`
	Kind        string      `json:"kind"`
	Title       string      `json:"title"`
	Clause      string      `json:"clause"`
	Text        string      `json:"text"`
	Source      Source      `json:"source"`
	AppliesTo   []string    `json:"applies_to"`
	Scope       Scope       `json:"scope"`
	Status      string      `json:"status"`
	Supersedes  []string    `json:"supersedes"`
	Contradicts []string    `json:"contradicts"`
	Evidence    []string    `json:"evidence"`
	Tier        string      `json:"tier"`
	Tally       Tally       `json:"tally"`
	ValidFrom   string      `json:"valid_from"`
	InvalidAt   string      `json:"invalid_at"`
	Steps       []Step      `json:"steps"`
	Invariants  []Invariant `json:"invariants"`
	Predicates  []Predicate `json:"predicates"`
}

// File is one source's rows as written under Dir.
type File struct {
	Schema int   `json:"schema"`
	Rows   []Row `json:"rows"`
}

// prefixes key a node id by kind.
var prefixes = map[string]string{KindDecision: "dec", KindLesson: "les", KindIncident: "inc", KindConvention: "conv", KindException: "exc", KindFlow: "flow", KindObjective: "obj"}

// NodeID is the store's id for a row: the kind's prefix and the source's
// own id (les:B227, dec:D161).
func NodeID(kind, id string) graph.ID { return graph.ID(prefixes[kind] + ":" + id) }

// ReadDir reads every .json file under root/Dir in name order; a missing
// directory is no rows; a file under another schema is an error.
func ReadDir(root string) ([]Row, error) {
	es, err := os.ReadDir(filepath.Join(root, Dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Row
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, Dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var f File
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("knowledge: %s: %w", e.Name(), err)
		}
		if f.Schema != Schema {
			return nil, fmt.Errorf("knowledge: %s: schema %d, want %d; import again", e.Name(), f.Schema, Schema)
		}
		out = append(out, f.Rows...)
	}
	return out, nil
}

// WriteFile writes rows as name.json under root/Dir, every list present,
// and returns the path. The bytes depend on the rows alone.
func WriteFile(root, name string, rows []Row) (string, error) {
	if rows == nil {
		rows = []Row{}
	}
	for i := range rows {
		fill(&rows[i])
	}
	b, err := json.MarshalIndent(File{Schema: Schema, Rows: rows}, "", " ")
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name+".json")
	return p, os.WriteFile(p, append(b, '\n'), 0o644)
}

// fill makes every list of r present, so a reader of the file sees the
// whole schema.
func fill(r *Row) {
	r.Schema = Schema
	r.AppliesTo, r.Supersedes, r.Contradicts, r.Evidence = or(r.AppliesTo), or(r.Supersedes), or(r.Contradicts), or(r.Evidence)
	r.Scope.Paths, r.Scope.Languages = or(r.Scope.Paths), or(r.Scope.Languages)
	r.Steps, r.Invariants, r.Predicates = or(r.Steps), or(r.Invariants), or(r.Predicates)
}

func or[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

var prov = graph.Prov{Extractor: "knowledge", Version: "1"}

// Records is the store's records for rows: a node per row of a known kind
// that is not retired or superseded, at the row's tier, with its title,
// text, clause, source file, status and tier as attributes, and an
// applies_to edge per target whose global id names this repository (rid).
// Another repository's targets wait for the workspace (M6); an id that is
// not global is ignored.
func Records(rows []Row, rid string, rev graph.Rev) []graph.Record {
	var out []graph.Record
	for _, r := range rows {
		if r.ID == "" || prefixes[r.Kind] == "" || r.Status == StatusRetired || r.Status == StatusSuperseded {
			continue
		}
		id := NodeID(r.Kind, r.ID)
		tier := graph.Inferred
		if r.Tier == TierDeclared {
			tier = graph.Declared
		}
		out = append(out, graph.Record{Node: &graph.Node{ID: id, Kind: r.Kind, Tier: tier, Conf: 1.0, Rev: rev, Prov: prov,
			Attrs: map[string]string{"title": r.Title, "text": r.Text, "clause": r.Clause, "source": r.Source.File, "status": r.Status, "tier": r.Tier}}})
		for _, t := range r.AppliesTo {
			trid, local, ok := graph.SplitGlobal(graph.ID(t))
			if !ok || trid != rid {
				continue
			}
			out = append(out, graph.Record{Edge: &graph.Edge{Src: id, Pred: graph.AppliesTo, Dst: local, Tier: tier, Conf: 1.0, Rev: rev, Prov: prov}})
		}
	}
	return out
}
