package store

import (
	"os"
	"path/filepath"
)

// The steer router's once-per-session notices (card VERSION-GATE1). The
// first time a session's note is shadowed because its channel is not
// verified, or is held out, the router prints one user-only line; the
// marker is how a later call knows the session has had it. Like the stale
// trigger's, a marker is an empty file written without a sync: a crash that
// loses one prints the line once more, and nothing else.

// TakeNotice creates the session's marker name, FileNoticeChannel or
// FileNoticeHoldout, and reports whether this call created it: true once per
// session and marker, whoever races for it, and false after. Any other name,
// and any error, is false, which prints no notice.
func (s *Store) TakeNotice(sessionID, name string) bool {
	if name != FileNoticeChannel && name != FileNoticeHoldout || s.opened() != nil {
		return false
	}
	f, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

// ReturnNotice removes the session's marker name, for a call that took it
// and then could not write the line that records the notice, so a later call
// prints it.
func (s *Store) ReturnNotice(sessionID, name string) {
	if name == FileNoticeChannel || name == FileNoticeHoldout {
		_ = os.Remove(filepath.Join(s.RunDir(sessionID), name))
	}
}
