package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The stale trigger's hint (card STALE-NUDGE1, D166). The post of a gate run
// arms the session; the post of a write judges the run only while it is
// armed, and disarms it when it does, so the whole-run read the judge costs
// is paid about once per gate run rather than once per write. The marker is
// a hint, never truth: the judge reads the record, so a marker that drifted
// from it costs one spare judge or one missed nudge until the next gate run,
// never a false nudge. It is a file rather than a record for the reason the
// probe marker is: one stat answers it. It holds no bytes, and it is written
// without a sync, because a crash that loses it is a missed nudge.

// ArmStale arms the session's stale trigger.
func (s *Store) ArmStale(sessionID string) error {
	if err := s.opened(); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), FileStaleArmed), os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	return f.Close()
}

// StaleArmed reports whether the session's stale trigger is armed: one
// Lstat, which is all an unarmed write pays.
func (s *Store) StaleArmed(sessionID string) (bool, error) {
	_, err := os.Lstat(filepath.Join(s.RunDir(sessionID), FileStaleArmed))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// TakeStaleArm disarms the session's stale trigger and reports whether this
// call is the one that did. Two posts that race for one arm both see it
// armed, and only one takes it, so only one judges.
func (s *Store) TakeStaleArm(sessionID string) bool {
	return os.Remove(filepath.Join(s.RunDir(sessionID), FileStaleArmed)) == nil
}
