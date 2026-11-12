package store

import "errors"

// DeclareSeq is AppendDeclaration returning the seq the ordered stream gave
// the record, in LandExecution's shape: the seq on success and on
// ErrRecordDegraded, which fitLine reports after the line is written, and 0
// with the error otherwise (a lock that timed out, an unfittable record, a
// write that failed). A caller that cites the seq in another file (an overlay
// group's marker) therefore never cites one the ordered stream does not hold.
func (s *Store) DeclareSeq(rec Declaration) (int64, error) {
	var declared int64
	err := s.appendOrdered(rec.SessionID, func(seq int64) any {
		declared = seq
		rec.Seq = seq
		return rec
	})
	if err != nil && !errors.Is(err, ErrRecordDegraded) {
		return 0, err
	}
	return declared, err
}
