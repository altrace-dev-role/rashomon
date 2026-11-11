package graph

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
)

// Overlay groups in the session log (W-M2.5b). An edit's overlay is a group
// of records written in one append, closed by a marker as its last line. The
// hook appends only what the language resolver produced; the retractions an
// overlay implies are computed at replay, where the code index is loaded
// (ApplyOverlayDigest). A group with no marker, or fewer records than the
// marker counts, is dropped whole: the overlay says less, never wrong.

// Overlay group constants.
const (
	// KindOverlayGroup is the marker node's kind. It is never asserted into
	// a Store; the loader reads it and keeps the group.
	KindOverlayGroup = "overlay_group"
	// OverlayDeclared is a group written at act_declared, before the tool.
	OverlayDeclared = "declared"
	// OverlayLanded is a group written at act_landed, after it.
	OverlayLanded = "landed"
	// MaxOverlayRecords bounds the records of one group, its marker excluded.
	MaxOverlayRecords = 400
	// MaxOverlayBytes bounds the overlay bytes one session appends. The
	// counter lives in per-session state (OverlayCounter), never in the file.
	MaxOverlayBytes = 4 << 20
)

// ErrOverlayFull is the bound's answer: nothing was written, and the caller
// records overlay_journal_full.
var ErrOverlayFull = errors.New("graph: the session's overlay bytes are at the bound (overlay_journal_full)")

// OverlayGroup is one edit's overlay as the log holds it. Recs are the
// resolver's records. A landed group with no Recs and an OfSeq promotes the
// declared group of that seq: the landed content had the declared digest.
type OverlayGroup struct {
	Session, ToolUseID, Kind string
	File                     ID
	Digest, Commit           string
	Seq, OfSeq               int64
	// Carried is the predicate set of the extractor that produced Recs. The
	// marker records it and a replay passes it back unchanged, so a replay
	// retracts what the live write retracted.
	Carried Carried
	Recs    []Record
}

// BuildOverlayGroup is g's records, capped at MaxOverlayRecords (the file
// node then carries _cut=true and the declarations past the cap are left
// out with their edges), then the closing marker.
func BuildOverlayGroup(g OverlayGroup) []Record {
	recs := capOverlay(g.Recs)
	rev := RevOverlay(g.Commit, g.Seq)
	a := map[string]string{"session_id": g.Session, "file": string(g.File), "kind": g.Kind, "seq": strconv.FormatInt(g.Seq, 10),
		"tool_use_id": g.ToolUseID, "digest": g.Digest, "count": strconv.Itoa(len(recs)), "carried": g.Carried.String()}
	if g.OfSeq > 0 {
		a["of_seq"] = strconv.FormatInt(g.OfSeq, 10)
	}
	m := Record{Node: &Node{ID: ID("ovgroup:" + g.Session + ":" + a["seq"] + ":" + g.Kind), Kind: KindOverlayGroup, Attrs: a,
		Tier: Observed, Conf: 1, Status: Asserted, Rev: rev, Prov: Prov{Extractor: "session", Version: "v1"}}}
	return append(recs, m)
}

// capOverlay keeps the first records of recs up to MaxOverlayRecords: every
// non-sym node, then sym nodes in order with their edges, a sym's edge
// leaving with it. It copies the file node it marks.
func capOverlay(recs []Record) []Record {
	if len(recs) <= MaxOverlayRecords {
		return recs
	}
	budget := MaxOverlayRecords
	dropped := map[ID]bool{}
	var out []Record
	for _, r := range recs {
		switch {
		case r.Node == nil:
		case r.Node.Kind != "sym":
			out, budget = append(out, r), budget-1
		case budget >= 3:
			out, budget = append(out, r), budget-2
		default:
			dropped[r.Node.ID] = true
		}
	}
	for _, r := range recs {
		if r.Edge != nil && !dropped[r.Edge.Dst] && !dropped[r.Edge.Src] {
			out = append(out, r)
		}
	}
	for i, r := range out {
		if r.Node != nil && r.Node.Kind == "file" {
			n := *r.Node
			n.Attrs = map[string]string{"_cut": "true"}
			for k, v := range r.Node.Attrs {
				n.Attrs[k] = v
			}
			out[i].Node = &n
			break
		}
	}
	return out
}

// OverlayCounter is the arithmetic of the per-session bound. The state it
// stands for is persisted by the writer inside its ordered-append critical
// section; this type holds no file.
type OverlayCounter struct{ Used int64 }

// Reserve adds n bytes, or returns ErrOverlayFull and adds nothing.
func (c *OverlayCounter) Reserve(n int64) error {
	if c.Used+n > MaxOverlayBytes {
		return ErrOverlayFull
	}
	c.Used += n
	return nil
}

// AppendOverlayGroup writes a built group (BuildOverlayGroup) in one
// append. The bytes are measured from the records, reserve(n) is called with
// them before any file is touched, and the append happens only when reserve
// returns nil. A nil reserve is no bound.
func (l *SessionLog) AppendOverlayGroup(recs []Record, reserve func(n int64) error) error {
	if !l.allowed {
		return ErrNoSessionConsent
	}
	var n int64
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		n += int64(len(b)) + 1
	}
	if reserve != nil {
		if err := reserve(n); err != nil {
			return err
		}
	}
	return l.Append(recs...)
}

// overlayCollector holds the overlay-revision records read since the last
// break in the file (an unreadable line, a header or a record at any other
// revision) and closes them into a group at a marker.
type overlayCollector struct {
	pending []Record
	groups  []OverlayGroup
}

func (c *overlayCollector) reset() { c.pending = nil }

// take consumes r when it is at an overlay revision and reports so; any
// other record breaks the run and is left to the caller.
func (c *overlayCollector) take(r Record) bool {
	var rev Rev
	switch {
	case r.Node != nil:
		rev = r.Node.Rev
	case r.Edge != nil:
		rev = r.Edge.Rev
	}
	if rev.Kind != RevKindOverlay {
		c.reset()
		return false
	}
	if r.Node == nil || r.Node.Kind != KindOverlayGroup {
		c.pending = append(c.pending, r)
		return true
	}
	a := r.Node.Attrs
	count, cerr := strconv.Atoi(a["count"])
	seq, serr := strconv.ParseInt(a["seq"], 10, 64)
	if cerr == nil && serr == nil && count >= 0 && count <= len(c.pending) {
		g := OverlayGroup{Session: a["session_id"], ToolUseID: a["tool_use_id"], Kind: a["kind"], File: ID(a["file"]),
			Digest: a["digest"], Commit: rev.Commit, Seq: seq, Carried: ParseCarried(a["carried"])}
		g.OfSeq, _ = strconv.ParseInt(a["of_seq"], 10, 64)
		g.Recs = append([]Record(nil), c.pending[len(c.pending)-count:]...)
		c.groups = append(c.groups, g)
	}
	c.pending = nil
	return true
}

// ReplayOverlays applies, to s, this session's groups at commit in seq
// order and returns how many it applied. s holds the code index and the
// session records. A declared group is dropped when its call failed (the
// call node carries a non-zero exit) or when a landed group for the same
// tool call exists, and a landed group with no records of its own applies its
// declared group's records at the landed seq.
func ReplayOverlays(s *Store, groups []OverlayGroup, session, commit string) int {
	var mine []OverlayGroup
	landed := map[string]bool{}
	declared := map[int64]OverlayGroup{}
	for _, g := range groups {
		if g.Session != session || g.Commit != commit {
			continue
		}
		mine = append(mine, g)
		if g.Kind == OverlayLanded {
			landed[g.ToolUseID] = true
		} else {
			declared[g.Seq] = g
		}
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].Seq < mine[j].Seq })
	applied := 0
	for _, g := range mine {
		recs := g.Recs
		if g.Kind == OverlayDeclared && (landed[g.ToolUseID] && g.ToolUseID != "" || callFailed(s, session, g.ToolUseID, commit)) {
			continue
		}
		if g.Kind == OverlayLanded && len(recs) == 0 {
			d, ok := declared[g.OfSeq]
			if !ok || d.File != g.File {
				continue
			}
			recs, g.Carried = d.Recs, d.Carried
		}
		if len(recs) == 0 {
			continue
		}
		s.ApplyOverlayDigest(g.File, g.Digest, recs, RevOverlay(commit, g.Seq), g.Carried)
		applied++
	}
	return applied
}

// callFailed reports whether the session's call for toolUseID is in s with a
// non-zero exit status. An absent call, or one with no status, is not failed.
func callFailed(s *Store, session, toolUseID, commit string) bool {
	n, ok := s.Node(CallNodeID(session, toolUseID), RevCommit(commit))
	if !ok {
		return false
	}
	exit, has := n.Attrs["exit"]
	return has && exit != "0"
}
