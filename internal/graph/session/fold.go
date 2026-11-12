package session

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// Event is one record line or steer line, decoded to its place in the
// session's merged order and the fields a fold rule reads, nothing else: a
// line's words (model_text, user_text) and every field no rule reads are
// never decoded, so they cannot reach a frontier (TestEvent_HoldsNoWords).
type Event struct {
	// Seq is a record's seq, or a steer line's after_seq: the last record
	// seq the line's decision saw.
	Seq int64
	// Steer is true for a steer line. At one seq a record sorts first,
	// because the line came after the record its after_seq names.
	Steer bool
	// AtUnixMS and ID are a steer line's clock and id, the order among the
	// lines at one seq. A record leaves them zero.
	AtUnixMS int64
	ID       string
	// Type is the record's type, or steer.LineType.
	Type string
	// State, Key and Kind are a steer line's state (steer.LineState), its
	// finding's dedupe key and its kind, which the Delivered rule reads. A
	// record leaves them empty.
	State string
	Key   string
	Kind  string
	// PromptID is a steer line's prompt_id, an id the Intents rule reads;
	// a line with no prompt (a briefing: prompt_id null) leaves it empty.
	PromptID string
}

// Before reports whether e sorts before o in the merged order (seq, record
// before line, at_unix_ms, id). Record seqs are unique (nextSeq allocates
// them under the records lock) and so are line ids, so the order is total.
func (e Event) Before(o Event) bool {
	if e.Seq != o.Seq {
		return e.Seq < o.Seq
	}
	if e.Steer != o.Steer {
		return !e.Steer
	}
	if e.AtUnixMS != o.AtUnixMS {
		return e.AtUnixMS < o.AtUnixMS
	}
	return e.ID < o.ID
}

var errUnplaced = errors.New("session: a line the merge cannot place")

// RecordEvent decodes one records.ndjson line. Every record with a type and
// a seq is an event; a rule that has no use for its type passes it by.
func RecordEvent(line []byte) (ev Event, ok bool, err error) {
	var h struct {
		Type string `json:"type"`
		Seq  *int64 `json:"seq"`
	}
	if err := json.Unmarshal(line, &h); err != nil {
		return Event{}, false, fmt.Errorf("session: record line: %w", err)
	}
	if h.Type == "" || h.Seq == nil {
		return Event{}, false, fmt.Errorf("%w: record without type or seq", errUnplaced)
	}
	return Event{Seq: *h.Seq, Type: h.Type}, true, nil
}

// SteerEvent decodes one steer.ndjson line. Only a steer.LineType line is an
// event: a steer_evidence line (steer.EvidenceLineType) carries no after_seq
// and feeds no frontier field, so ok is false and err nil; its bytes still
// move the steer cursor.
func SteerEvent(line []byte) (ev Event, ok bool, err error) {
	var h struct {
		Type     string  `json:"type"`
		ID       string  `json:"id"`
		AtUnixMS int64   `json:"at_unix_ms"`
		AfterSeq *int64  `json:"after_seq"`
		State    string  `json:"state"`
		Key      string  `json:"key"`
		Kind     string  `json:"kind"`
		PromptID *string `json:"prompt_id"`
	}
	if err := json.Unmarshal(line, &h); err != nil {
		return Event{}, false, fmt.Errorf("session: steer line: %w", err)
	}
	if h.Type != steer.LineType {
		return Event{}, false, nil
	}
	if h.AfterSeq == nil || h.ID == "" {
		return Event{}, false, fmt.Errorf("%w: steer line without after_seq or id", errUnplaced)
	}
	ev = Event{Seq: *h.AfterSeq, Steer: true, AtUnixMS: h.AtUnixMS, ID: h.ID, Type: h.Type, State: h.State, Key: h.Key, Kind: h.Kind}
	if h.PromptID != nil {
		ev.PromptID = *h.PromptID
	}
	return ev, true, nil
}

// Step applies one event, taken in the merged order, to f. It is pure: no
// I/O, no clock and no graph, so what it writes depends on f and ev alone,
// and a fold over a prefix then the tail equals a fold over the whole.
func Step(f *Frontier, ev Event) {
	if ev.PromptID != "" {
		intent(f, ev)
	}
	if ev.State == string(steer.LineStateDelivered) { // only SteerEvent sets State
		deliver(f, ev)
	}
}

// deliver keeps a delivered steer line as a LineRef at its after_seq. Only
// state delivered counts: suppressed, superseded, stale, expired and
// unconfirmed lines never reached the model as a delivery. One LineRef per
// line id (canon's order needs it): a line seen again keeps its first ref.
// Unit stays empty: only a graph-kind line names one, and at this commit
// no writer emits a graph-kind steer line (M1's brief is kind "", its
// graph lines inside its slots).
func deliver(f *Frontier, ev Event) {
	for _, r := range f.Delivered {
		if r.ID == ev.ID {
			return
		}
	}
	f.Delivered = append(f.Delivered, LineRef{ID: ev.ID, Key: ev.Key, Kind: ev.Kind, Seq: ev.Seq})
}

// intent keeps each prompt's first steer line, in any state, as an
// IntentRef (D2). n is the prompt's order of first appearance in the merged
// order, counted from 1 as the graph's prompt nodes are, so ID is that
// node's id; Digest is the prompt id, because no line carries a prompt
// digest; StatedAt is the line's after_seq; Status is open (v1 closes no
// intent). A prompt that wrote no steer line has no IntentRef and shifts
// every later n: the record holds no prompt line to count from.
func intent(f *Frontier, ev Event) {
	for _, r := range f.Intents {
		if r.Digest == ev.PromptID {
			return
		}
	}
	n := len(f.Intents) + 1
	f.Intents = append(f.Intents, IntentRef{ID: string(graph.PromptNodeID(f.Session, n)), Digest: ev.PromptID, StatedAt: ev.Seq, Status: ReqOpen})
}
