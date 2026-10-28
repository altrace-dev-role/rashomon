package store

import (
	"errors"
	"path/filepath"
)

// AppendExecution records that a declared call ran, allocating its seq. If the
// ordered stream's lock cannot be taken it falls back to the spill file: an
// execution is the only record its tool_use_id gets, so dropping it would read
// afterwards as a call that never ran.
func (s *Store) AppendExecution(rec Execution) error {
	_, err := s.LandExecution(rec)
	return err
}

// LandExecution is AppendExecution returning the seq the ordered stream gave
// the record, when it holds one: on success, and on ErrRecordDegraded, which
// fitLine reports after the line is written. A spilled, unfittable or
// unwritten execution returns 0 with the append's error, so a caller never
// cites a seq the ordered stream does not hold (card 2.3 R2).
func (s *Store) LandExecution(rec Execution) (int64, error) {
	var landed int64
	err := s.appendOrdered(rec.SessionID, func(seq int64) any {
		landed = seq
		rec.Seq = &seq
		return rec
	})
	switch {
	case errors.Is(err, ErrLockTimeout):
		return 0, s.SpillExecution(rec)
	case err != nil && !errors.Is(err, ErrRecordDegraded):
		return 0, err
	}
	return landed, err
}

// SpillExecution writes an execution to the spill file without touching the
// ordered stream, under the same rule as SpillTerminal.
func (s *Store) SpillExecution(rec Execution) error {
	rec.Seq = nil
	return s.appendLine(filepath.Join(s.RunDir(rec.SessionID), FileSpill), rec, spillBudget, appendWithoutLock)
}
