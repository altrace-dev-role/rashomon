package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// OverlayState is an overlay's standing against the next index.
type OverlayState string

// The overlay states: live until an index at a later commit compares the
// committed file's digest with the overlay's; equal is subsumed, different
// is stale.
const (
	OverlayLive     OverlayState = "live"
	OverlaySubsumed OverlayState = "subsumed"
	OverlayStale    OverlayState = "stale"
)

// Overlay is the record of one edit's view of one file, written at
// PreToolUse before the tool runs (contracts-v2 section 2.1).
type Overlay struct {
	ID      string
	File    ID
	Base    string
	Seq     int64
	Digest  string
	State   OverlayState
	Records int
}

// Record is one extractor output: a node or an edge, never both.
type Record struct {
	Node *Node
	Edge *Edge
}

// ContentDigest is the digest an overlay and an index compare.
func ContentDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ApplyOverlay writes an edit's records for file at rev (an overlay
// revision), retracting at the same revision every symbol the file declared
// at the revision before this one that the edit no longer declares, with
// that symbol's outgoing edges, and every outgoing edge of a kept symbol the
// edit no longer produces. Edges into the file's symbols from other files
// are left standing; the conflict step reads them against the new
// signatures. The Overlay record it returns is appended to the store.
func (s *Store) ApplyOverlay(file ID, content []byte, recs []Record, rev Rev) Overlay {
	prev := Rev{Kind: RevKindOverlay, Commit: rev.Commit, Seq: rev.Seq - 1}
	kept := map[ID]bool{}
	produced := map[string]bool{}
	for _, r := range recs {
		if r.Node != nil {
			kept[r.Node.ID] = true
		}
		if r.Edge != nil {
			produced[r.Edge.Key()] = true
		}
	}
	for _, d := range s.Out(file, prev, Declares) {
		if !kept[d.Dst] {
			s.RetractNode(d.Dst, rev)
			s.RetractEdge(d.Src, d.Pred, d.Dst, rev)
		}
		for _, e := range s.Out(d.Dst, prev) {
			if !produced[e.Key()] {
				s.RetractEdge(e.Src, e.Pred, e.Dst, rev)
			}
		}
	}
	for _, e := range s.Out(file, prev) {
		if !produced[e.Key()] {
			s.RetractEdge(e.Src, e.Pred, e.Dst, rev)
		}
	}
	for _, r := range recs {
		if r.Node != nil {
			n := *r.Node
			n.Rev = rev
			s.AssertNode(n)
		}
		if r.Edge != nil {
			e := *r.Edge
			e.Rev = rev
			s.AssertEdge(e)
		}
	}
	ov := Overlay{ID: "ov:" + strconv.FormatInt(rev.Seq, 10), File: file, Base: rev.Commit, Seq: rev.Seq,
		Digest: ContentDigest(content), State: OverlayLive, Records: len(recs)}
	s.overlays = append(s.overlays, ov)
	return ov
}

// Overlays is every overlay written, in order.
func (s *Store) Overlays() []Overlay { return append([]Overlay(nil), s.overlays...) }

// Reconcile is the index step at commit: each live overlay whose digest
// equals the committed digest of its file is subsumed, the rest are stale.
// The committed records at commit are written by the caller; this only
// settles the overlays' states.
func (s *Store) Reconcile(commit string, digests map[ID]string) {
	for i := range s.overlays {
		ov := &s.overlays[i]
		if ov.State != OverlayLive {
			continue
		}
		if digests[ov.File] == ov.Digest {
			ov.State = OverlaySubsumed
		} else {
			ov.State = OverlayStale
		}
	}
}
