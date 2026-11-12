package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// FrontierSchema is the frontier's own schema, written first as "schema".
const FrontierSchema = 1

// ReqStatus is where a requirement stands. Transitions are monotone: open to
// met or superseded, never back (11-05 order, PLAN 1478).
type ReqStatus string

const (
	ReqOpen       ReqStatus = "open"
	ReqMet        ReqStatus = "met"
	ReqDropped    ReqStatus = "dropped"
	ReqSuperseded ReqStatus = "superseded"
)

// Frontier is a session's state folded from its record and steer log: a
// pure function of them, so deleting a saved one changes no answer, only its
// cost (proposal 2026-11-12 section 0; ruling Q1). Every string in it is an
// id, a digest or a closed name, never prompt or file words
// (TestFrontier_WordsByReference). It holds no map, so its JSON is its
// canonical order (MarshalFrontier).
//
// Differences from PLAN 1478's interface, each for a reason: no Repo and no
// Commit, because no record line carries a repo id or the session's start
// commit, and a field the record cannot give breaks C1; no Drift until
// W-M4.6, which adds it under a fold_version bump; ObligationsAt and
// ObligationsOmitted, ledger conditions 4 and 3 (ruling 09:53); Touched
// holds files, not units, for the same reason (its doc); TouchedWithheld,
// condition 3's cut rule applied to Touched, and TouchedOutside, the writes
// Touched cannot place in the project (ruling 11:37).
type Frontier struct {
	Schema    int    `json:"schema"`
	Session   string `json:"session"`
	Continues string `json:"continues"`
	// Intents are the session's prompts by reference: graph.PromptNodeID
	// and the prompt digest.
	Intents   []IntentRef   `json:"intents"`
	Reqs      []ReqState    `json:"reqs"`
	Decisions []DecisionRef `json:"decisions"`
	// Touched are file ids: the graph.FileID of each project-relative path
	// the session's calls named as written, sorted, unique (D1). Written
	// only, never a read: the record names a path only for a file-changing
	// call (shape.DeclaredPath) or a shell line's written_paths. Files, not
	// units: a unit id needs the language's resolver, and pylite's units
	// come from the index, not the record (C1). The unit form and the global
	// form are the reader's, made at read time: lang.Registry maps a file to
	// its unit, and the repo id makes the global id (graph.GlobalID).
	Touched []string `json:"touched"`
	// TouchedWithheld counts write declarations, not files: each declaration
	// whose write path the record withheld counts once (Event.WroteWithheld),
	// whatever the reason (no_consent below record scope v5, oversize,
	// switched_off). Below v5 the gate nulls a shell line's written_paths
	// whole and keys it no_consent only when the line named a path
	// (store.GateDeclaration), so a Bash line that named three paths counts
	// 1, and one that named none holds no key and counts 0. When it is above
	// zero Touched is not every file the calls named, and nothing built from
	// it may say every. Touched is never every file written in any case: a
	// shell line that may write names no path when its written_paths list is
	// incomplete (verify's unnamed writes).
	TouchedWithheld int `json:"touched_withheld"`
	// TouchedOutside counts write declarations, not files, that named a
	// path outside the project: still absolute after the project and cwd
	// prefixes are cut, climbing out, or on another drive (inProject). A
	// declaration with paths on both sides adds its inside ones to Touched
	// and counts once here. No symlink is resolved, so a path written
	// through an alias of the project (/tmp for /private/tmp) is outside: a
	// session launched by an alias has an empty Touched and every write
	// here. Both counts at zero still do not make an empty Touched "wrote
	// nothing": a shell line that may write names no path when its list is
	// incomplete, so a reader says at most "named no written file" (ruling
	// 11:46).
	TouchedOutside int `json:"touched_outside"`
	// Obligations are copied from the last ledger line, as of its seq
	// (ObligationsAt); the fold never computes one. ObligationsOmitted is
	// the ledger line's own cut count: when it is above zero the list is not
	// every obligation, and nothing built from it may say every.
	Obligations        []ObligationRef `json:"obligations"`
	ObligationsAt      int64           `json:"obligations_at"`
	ObligationsOmitted int             `json:"obligations_omitted"`
	Delivered          []LineRef       `json:"delivered"`
	Verdicts           []VerdictRef    `json:"verdicts"`
	LastBoundary       int64           `json:"last_boundary"`
}

// IntentRef is one prompt by reference.
type IntentRef struct {
	ID       string    `json:"id"`
	Digest   string    `json:"digest"`
	StatedAt int64     `json:"stated_at"`
	Status   ReqStatus `json:"status"`
}

// ReqState is a requirement by its predicate's name and argument, so a
// frontier is printed, compared and re-delivered without the store (AD17).
type ReqState struct {
	ID           string    `json:"id"`
	Pred         string    `json:"pred"`
	Arg          string    `json:"arg"`
	Status       ReqStatus `json:"status"`
	StatedAt     int64     `json:"stated_at"`
	MetAt        int64     `json:"met_at"`
	SupersededBy string    `json:"superseded_by"`
}

// DecisionRef is a delivered decision line: the ids it applies to and what
// it asks of them.
type DecisionRef struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action"`
}

// ObligationRef is one ledger row as the ledger line carried it.
type ObligationRef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Unit  string `json:"unit"`
	State string `json:"state"`
}

// LineRef is a steer line the session was delivered.
type LineRef struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Unit string `json:"unit"`
	Seq  int64  `json:"seq"`
}

// VerdictRef is a Stop's verdict, by the seq of its checkpoint record.
type VerdictRef struct {
	Seq  int64 `json:"seq"`
	Done bool  `json:"done"`
	Owed int   `json:"owed"`
}

func newFrontier(sessionID string) Frontier {
	return Frontier{Schema: FrontierSchema, Session: sessionID}
}

// canon returns f with every slice in its canonical order and every nil
// slice empty, so one state has one encoding. It copies; f is not changed.
// The order is total only while every sort key is unique, which the fold
// keeps: one ReqState per line key, one VerdictRef per checkpoint seq, one
// LineRef per steer line id. Two elements equal on the key and different
// elsewhere would let two input orders encode differently.
func canon(f Frontier) Frontier {
	g := f
	g.Intents = append([]IntentRef{}, f.Intents...)
	sort.Slice(g.Intents, func(i, j int) bool {
		a, b := g.Intents[i], g.Intents[j]
		return a.StatedAt < b.StatedAt || a.StatedAt == b.StatedAt && a.ID < b.ID
	})
	g.Reqs = append([]ReqState{}, f.Reqs...)
	sort.Slice(g.Reqs, func(i, j int) bool {
		a, b := g.Reqs[i], g.Reqs[j]
		return a.StatedAt < b.StatedAt || a.StatedAt == b.StatedAt && a.ID < b.ID
	})
	g.Decisions = make([]DecisionRef, len(f.Decisions))
	for i, d := range f.Decisions {
		ids := append([]string{}, d.IDs...)
		sort.Strings(ids)
		g.Decisions[i] = DecisionRef{IDs: ids, Action: d.Action}
	}
	sort.Slice(g.Decisions, func(i, j int) bool {
		a, b := strings.Join(g.Decisions[i].IDs, "\x00"), strings.Join(g.Decisions[j].IDs, "\x00")
		return a < b || a == b && g.Decisions[i].Action < g.Decisions[j].Action
	})
	g.Touched = append([]string{}, f.Touched...)
	sort.Strings(g.Touched)
	g.Obligations = append([]ObligationRef{}, f.Obligations...)
	sort.Slice(g.Obligations, func(i, j int) bool { return g.Obligations[i].ID < g.Obligations[j].ID })
	g.Delivered = append([]LineRef{}, f.Delivered...)
	sort.Slice(g.Delivered, func(i, j int) bool {
		a, b := g.Delivered[i], g.Delivered[j]
		return a.Seq < b.Seq || a.Seq == b.Seq && a.ID < b.ID
	})
	g.Verdicts = append([]VerdictRef{}, f.Verdicts...)
	sort.Slice(g.Verdicts, func(i, j int) bool { return g.Verdicts[i].Seq < g.Verdicts[j].Seq })
	return g
}

// MarshalFrontier encodes f in its canonical order: two orders of one state
// give the same bytes.
func MarshalFrontier(f Frontier) ([]byte, error) {
	return json.Marshal(canon(f))
}

// ParseFrontier decodes a frontier of FrontierSchema. Any other schema, an
// unknown field or trailing data is an error, which a cache reader takes as
// a miss.
func ParseFrontier(b []byte) (Frontier, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f Frontier
	if err := dec.Decode(&f); err != nil {
		return Frontier{}, err
	}
	// More would answer false at a stray } or ], which ends no container
	// here; Token reports io.EOF only when nothing but space is left.
	if _, err := dec.Token(); err != io.EOF {
		return Frontier{}, fmt.Errorf("frontier: data after the object")
	}
	if f.Schema != FrontierSchema {
		return Frontier{}, fmt.Errorf("frontier: schema %d, want %d", f.Schema, FrontierSchema)
	}
	return canon(f), nil
}
