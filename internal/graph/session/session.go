package session

import (
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
)

// Via is the channel a fact reached the agent by (contracts-v2 section
// 4.1). Only wrote is the agent's own act; the others prove delivery, not
// receipt.
type Via string

// The channels.
const (
	ViaContext Via = "context"
	ViaMessage Via = "message"
	ViaOpened  Via = "opened"
	ViaWrote   Via = "wrote"
)

// KnowState is what the session can say about a delivered fact.
type KnowState string

// The states. Sent proves nothing beyond the channel; demonstrated means an
// act after delivery is consistent with the fact; lost means a boundary
// passed with no demonstrating act after it.
const (
	Sent         KnowState = "sent"
	Demonstrated KnowState = "demonstrated"
	Contradicted KnowState = "contradicted"
	Lost         KnowState = "lost"
)

// Know is one record of a fact reaching the agent.
type Know struct {
	ID    string
	Fact  string
	Via   Via
	AtSeq int64
	State KnowState
	By    int64
	Rev   string
}

// Req is one requirement: its words when the person stated them, its
// predicate over the graph when it has one, and whether it holds now.
type Req struct {
	ID    string
	Words *Ret
	Tier  graph.Tier
	Conf  float64
	Text  string
	Pred  func(*graph.Store, graph.Rev) bool
	Holds bool
}

// Gate is one declared gate: its command, the unit it covers ("" for the
// whole checkout), and the seq of its last run.
type Gate struct {
	Cmd    string
	Unit   string
	RanAt  int64
	Passed bool
}

// Touch is one write.
type Touch struct {
	Path string
	Seq  int64
}

// Decision is one held decision: the words by reference and the project
// names they resolved to, kept in clear.
type Decision struct {
	Words *Ret
	Names []string
}

// ConflictState is a conflict's standing in the session.
type ConflictState string

// The conflict states: pending until Stop or resolution; owed at Stop when
// still standing; resolved when a later overlay cleared it.
const (
	Pending  ConflictState = "pending"
	Resolved ConflictState = "resolved"
	Owed     ConflictState = "owed"
)

// Conflict is an engine conflict with its session state.
type Conflict struct {
	engine.Conflict
	State ConflictState
	First int64
}

// Session is the session graph for one task.
type Session struct {
	ID        string
	Store     *graph.Store
	Commit    string
	Goal      *Ret
	Reqs      []*Req
	Decisions []Decision
	Gates     []*Gate
	Delivered []*Delivered
	Touched   []Touch
	Knows     []*Know
	Conflicts map[string]*Conflict
	Boundary  int
	Seq       int64
	Digest    func(string) string
	Extract   func([]byte) (string, bool)
	CapBytes  int64
}

// New is an empty session over store at commit.
func New(id string, store *graph.Store, commit string) *Session {
	return &Session{ID: id, Store: store, Commit: commit, Conflicts: map[string]*Conflict{}}
}

// Rev is the revision the session reads at: the commit plus every overlay
// through the last write.
func (s *Session) Rev() graph.Rev {
	if len(s.Touched) == 0 {
		return graph.RevCommit(s.Commit)
	}
	return graph.RevOverlay(s.Commit, s.Touched[len(s.Touched)-1].Seq)
}

// Deliver records facts sent through the context channel at seq.
func (s *Session) Deliver(seq int64, ids ...string) {
	s.advance(seq)
	for _, id := range ids {
		s.Knows = append(s.Knows, &Know{ID: "know:" + s.ID + ":" + id, Fact: id, Via: ViaContext, AtSeq: seq, State: Sent, Rev: s.Rev().String()})
	}
}

// Opened records a Read of path at seq.
func (s *Session) Opened(seq int64, path string) {
	s.advance(seq)
	s.Knows = append(s.Knows, &Know{ID: "know:" + s.ID + ":open:" + path, Fact: string(graph.FileID(path)), Via: ViaOpened, AtSeq: seq, State: Sent, Rev: s.Rev().String()})
}

// Wrote records a write of path at seq whose overlay is already in the
// store: the touch, the know record, the conflicts of every symbol the
// file now declares, the re-check of every pending conflict, the
// requirements, and which delivered facts the write demonstrates.
func (s *Session) Wrote(seq int64, path string) {
	s.advance(seq)
	s.Touched = append(s.Touched, Touch{Path: path, Seq: seq})
	rev := s.Rev()
	s.Knows = append(s.Knows, &Know{ID: "know:" + s.ID + ":wrote:" + path, Fact: string(graph.FileID(path)), Via: ViaWrote, AtSeq: seq, State: Sent, Rev: rev.String()})
	for _, d := range s.Store.Out(graph.FileID(path), rev, graph.Declares) {
		for _, c := range append(engine.ArityConflicts(s.Store, d.Dst, rev), engine.CallerConflicts(s.Store, d.Dst, rev)...) {
			if _, seen := s.Conflicts[c.Key()]; !seen {
				s.Conflicts[c.Key()] = &Conflict{Conflict: c, State: Pending, First: seq}
			}
		}
	}
	for k, c := range s.Conflicts {
		if c.State != Pending {
			continue
		}
		still := false
		for _, now := range engine.ArityConflicts(s.Store, c.Edge.Dst, rev) {
			if now.Key() == k {
				still, c.Conflict = true, now
			}
		}
		if !still {
			c.State = Resolved
		}
	}
	for _, r := range s.Reqs {
		if r.Pred != nil {
			r.Holds = r.Pred(s.Store, rev)
		}
	}
	for _, d := range s.Delivered {
		if d.Pred != nil && d.Pred(s) == FollowedYes {
			s.demonstrate(d.ID, seq)
		}
	}
}

// Ran records a gate's run at seq.
func (s *Session) Ran(seq int64, cmd string, passed bool) {
	s.advance(seq)
	for _, g := range s.Gates {
		if g.Cmd == cmd {
			g.RanAt, g.Passed = seq, passed
		}
	}
}

// MarkBoundary records a compaction at seq: every know record delivered
// before it is lost, keeping the act that demonstrated it when one did.
func (s *Session) MarkBoundary(seq int64) {
	s.advance(seq)
	s.Boundary++
	for _, k := range s.Knows {
		if k.AtSeq < seq {
			k.State = Lost
		}
	}
}

// demonstrate moves the sent know records for fact to demonstrated by seq.
func (s *Session) demonstrate(fact string, seq int64) {
	for _, k := range s.Knows {
		if k.Fact == fact && k.State == Sent {
			k.State, k.By = Demonstrated, seq
		}
	}
}

// advance keeps the latest seq.
func (s *Session) advance(seq int64) {
	if seq > s.Seq {
		s.Seq = seq
	}
}

// KnowsIn is the know records in state, in order.
func (s *Session) KnowsIn(state KnowState) []*Know {
	var out []*Know
	for _, k := range s.Knows {
		if k.State == state {
			out = append(out, k)
		}
	}
	return out
}

// KnowFor is the first know record for fact.
func (s *Session) KnowFor(fact string) *Know {
	for _, k := range s.Knows {
		if k.Fact == fact {
			return k
		}
	}
	return nil
}

// ConflictsIn is the conflicts in state, by first seq then key.
func (s *Session) ConflictsIn(state ConflictState) []*Conflict {
	var out []*Conflict
	for _, c := range s.Conflicts {
		if c.State == state {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].First != out[j].First {
			return out[i].First < out[j].First
		}
		return out[i].Key() < out[j].Key()
	})
	return out
}

// owedGates is every gate with a write it covers after its last run, with
// the last such write.
func (s *Session) owedGates() []owedGate {
	var out []owedGate
	for _, g := range s.Gates {
		var last Touch
		for _, t := range s.Touched {
			if t.Seq > g.RanAt && (g.Unit == "" || t.Path == g.Unit || strings.HasPrefix(t.Path, g.Unit+"/")) {
				last = t
			}
		}
		if last.Seq > 0 {
			out = append(out, owedGate{Gate: g, Last: last})
		}
	}
	return out
}

// owedGate is a gate and the write that owes it.
type owedGate struct {
	Gate *Gate
	Last Touch
}
