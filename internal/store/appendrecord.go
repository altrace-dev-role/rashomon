package store

import "errors"

// ErrUnknownKind is AppendRecord's refusal of a record whose kind, at its
// version, AcceptsKind does not accept: a writer never puts on the record a
// line its own reader would count as unreadable.
var ErrUnknownKind = errors.New("store: record kind is not accepted at its version")

// SeqRecord is a record a package above the store builds and the store
// orders: it names its own kind and version, and takes the seq the store
// allocates under the run's lock. The interface lives here and its
// implementations in their callers (verify.Checkpoint), so the store imports
// none of them.
type SeqRecord interface {
	Kind() (string, int)
	SetSeq(*int64)
}

// AppendRecord appends rec to the session's records.ndjson as an ordered
// record: the seq is allocated and set inside the critical section, and
// fitLine's caps and the degraded rule apply to it as to every record. A kind
// AcceptsKind refuses is ErrUnknownKind, and nothing is written. A lock
// timeout is ErrLockTimeout and spills nothing: every record written here is
// derivable from the records before it, so losing one loses no fact, and a
// spilled copy without a seq would place nothing.
func (s *Store) AppendRecord(sessionID string, rec SeqRecord) error {
	if kind, version := rec.Kind(); !AcceptsKind(kind, version) {
		return ErrUnknownKind
	}
	return s.appendOrdered(sessionID, func(seq int64) any {
		rec.SetSeq(&seq)
		return rec
	})
}
