// Package graph holds the platform graph's records and the one reader every
// engine query goes through. It is the smallest build that proves the three
// contract fixes of research-2026-11-04-autosteer-contracts-v2: every record
// carries the revision it was computed at (a commit, or a commit plus an
// overlay sequence), every edge carries Certain, computed by one writer rule
// and never set by hand, and a contract id carries the scope of the
// component that exposes it, so two services on one route are two nodes.
//
// The store is in memory. A durable store, the full extractors and the
// engine's ranking are after this slice; nothing here reads a file, starts
// a process or opens a socket.
package graph

import (
	"sort"
	"strconv"
)

// ID is a node id in the grammar of contracts-v2 section 3.1; no position
// ever appears in one.
type ID string

// Pred is an edge predicate in the core vocabulary.
type Pred string

// The core predicates this slice reads. MayCall and MayResolveTo are
// predicates of their own, never projected onto Calls or ResolvesTo.
const (
	Contains        Pred = "contains"
	Declares        Pred = "declares"
	Imports         Pred = "imports"
	Calls           Pred = "calls"
	MayCall         Pred = "may_call"
	References      Pred = "references"
	Reads           Pred = "reads"
	Writes          Pred = "writes"
	Exposes         Pred = "exposes"
	Consumes        Pred = "consumes"
	ResolvesTo      Pred = "resolves_to"
	MayResolveTo    Pred = "may_resolve_to"
	CoChangedWith   Pred = "co_changed_with"
	DuplicatesValue Pred = "duplicates_value"
	Crosses         Pred = "crosses"
)

// Tier is how a record was produced.
type Tier string

// The tiers, strongest first.
const (
	Proven      Tier = "proven"
	Algorithmic Tier = "algorithmic"
	Declared    Tier = "declared"
	Observed    Tier = "observed"
	Inferred    Tier = "inferred"
)

// Status is a record's standing.
type Status string

// The statuses.
const (
	Asserted          Status = "asserted"
	Retracted         Status = "retracted"
	Unresolved        Status = "unresolved"
	NeedsRevalidation Status = "needs_revalidation"
)

// RevKind says whether a record was computed from a commit or from an edit
// that has not been committed.
type RevKind string

// The revision kinds.
const (
	RevKindCommit  RevKind = "commit"
	RevKindOverlay RevKind = "overlay"
)

// Rev is the revision a record was computed at: a commit, or a commit plus
// the store sequence of the edit whose overlay produced it. A reader at
// {C1, 16} sees C1 and every overlay with Seq <= 16.
type Rev struct {
	Kind   RevKind
	Commit string
	Seq    int64
}

// RevCommit is the committed view at c.
func RevCommit(c string) Rev { return Rev{Kind: RevKindCommit, Commit: c} }

// RevOverlay is the view at c plus every overlay through seq.
func RevOverlay(c string, seq int64) Rev {
	return Rev{Kind: RevKindOverlay, Commit: c, Seq: seq}
}

// Admits reports whether a reader at r sees a record computed at of.
func (r Rev) Admits(of Rev) bool {
	if of.Commit != r.Commit {
		return false
	}
	if of.Kind == RevKindCommit {
		return true
	}
	return r.Kind == RevKindOverlay && of.Seq <= r.Seq
}

// String is the form a delivered line carries: "C1" or "C1+ov16".
func (r Rev) String() string {
	if r.Kind == RevKindOverlay {
		return r.Commit + "+ov" + strconv.FormatInt(r.Seq, 10)
	}
	return r.Commit
}

// Prov is where a record came from. Evidence refs are ids of the records it
// was derived from, or of the declaration or run that backs it.
type Prov struct {
	Extractor string
	Version   string
	Evidence  []string
}

// Node is a node record. Attrs hold what the key does not: a symbol's
// signature, a constant's value, a consumer's base symbol.
type Node struct {
	ID     ID
	Kind   string
	Attrs  map[string]string
	Tier   Tier
	Conf   float64
	Status Status
	Rev    Rev
	Prov   Prov
}

// Edge is an edge record. Certain is set by the store on Assert (certain in
// basis.go) and is the one field the obligation writer reads.
type Edge struct {
	Src     ID
	Pred    Pred
	Dst     ID
	Attrs   map[string]string
	Tier    Tier
	Conf    float64
	Status  Status
	Rev     Rev
	Prov    Prov
	Certain bool
}

// Key is the edge's identity: one (src, pred, dst) has one history.
func (e Edge) Key() string { return string(e.Src) + "\x00" + string(e.Pred) + "\x00" + string(e.Dst) }

// Store is the in-memory record store: every version of every node and
// edge, in the order written, read back at a revision.
type Store struct {
	nodes    map[ID][]Node
	edges    map[string][]Edge
	out      map[ID][]string
	in       map[ID][]string
	overlays []Overlay
}

// NewStore is an empty store.
func NewStore() *Store {
	return &Store{nodes: map[ID][]Node{}, edges: map[string][]Edge{}, out: map[ID][]string{}, in: map[ID][]string{}}
}

// AssertNode writes a node version at n.Rev.
func (s *Store) AssertNode(n Node) {
	if n.Status == "" {
		n.Status = Asserted
	}
	s.nodes[n.ID] = append(s.nodes[n.ID], n)
}

// RetractNode writes a retraction of id at rev.
func (s *Store) RetractNode(id ID, rev Rev) {
	s.nodes[id] = append(s.nodes[id], Node{ID: id, Status: Retracted, Rev: rev})
}

// AssertEdge writes an edge version at e.Rev with Certain computed, never
// taken from the caller.
func (s *Store) AssertEdge(e Edge) {
	if e.Status == "" {
		e.Status = Asserted
	}
	e.Certain = certain(e)
	k := e.Key()
	if _, seen := s.edges[k]; !seen {
		s.out[e.Src] = append(s.out[e.Src], k)
		s.in[e.Dst] = append(s.in[e.Dst], k)
	}
	s.edges[k] = append(s.edges[k], e)
}

// RetractEdge writes a retraction of (src, pred, dst) at rev.
func (s *Store) RetractEdge(src ID, pred Pred, dst ID, rev Rev) {
	s.AssertEdge(Edge{Src: src, Pred: pred, Dst: dst, Status: Retracted, Rev: rev})
}

// Node is id's latest version a reader at rev admits, false when none, or
// when that version is a retraction.
func (s *Store) Node(id ID, at Rev) (Node, bool) {
	var best Node
	found := false
	for _, v := range s.nodes[id] {
		if at.Admits(v.Rev) && (!found || !later(best.Rev, v.Rev)) {
			best, found = v, true
		}
	}
	if !found || best.Status == Retracted {
		return Node{}, false
	}
	return best, true
}

// Out is the edges from id, at rev, with a predicate in preds (all when
// preds is empty), each the latest admitted version and not retracted,
// in a stable order.
func (s *Store) Out(id ID, at Rev, preds ...Pred) []Edge {
	return s.pick(s.out[id], at, preds)
}

// In is the edges into id, as Out.
func (s *Store) In(id ID, at Rev, preds ...Pred) []Edge {
	return s.pick(s.in[id], at, preds)
}

// Edge is the latest admitted version of (src, pred, dst), false when none
// or retracted.
func (s *Store) Edge(src ID, pred Pred, dst ID, at Rev) (Edge, bool) {
	es := s.pick([]string{Edge{Src: src, Pred: pred, Dst: dst}.Key()}, at, nil)
	if len(es) == 0 {
		return Edge{}, false
	}
	return es[0], true
}

// pick is the admitted, asserted edge for each key with a wanted predicate.
func (s *Store) pick(keys []string, at Rev, preds []Pred) []Edge {
	var out []Edge
	for _, k := range keys {
		var best Edge
		found := false
		for _, v := range s.edges[k] {
			if at.Admits(v.Rev) && (!found || !later(best.Rev, v.Rev)) {
				best, found = v, true
			}
		}
		if !found || best.Status == Retracted || !wanted(best.Pred, preds) {
			continue
		}
		out = append(out, best)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

// later reports whether a is a strictly newer revision than b: an overlay
// is newer than its commit, a higher overlay seq is newer than a lower.
// Among versions at one revision the last written wins.
func later(a, b Rev) bool {
	if a.Kind != b.Kind {
		return a.Kind == RevKindOverlay
	}
	return a.Seq > b.Seq
}

// wanted reports whether p is in preds, or preds is empty.
func wanted(p Pred, preds []Pred) bool {
	if len(preds) == 0 {
		return true
	}
	for _, q := range preds {
		if q == p {
			return true
		}
	}
	return false
}
