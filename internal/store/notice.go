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
// loses one prints the line once more, and nothing else. The Stop's coverage
// notice, and the line that says it has cleared, keep markers of the same
// kind (FileNoticeCoverage, FileNoticeCoverageClear), as does the prompt's
// line that the repository index is over its size cap (FileNoticeIndexCap).
// The manifest notice is owed by one hook and said by another: an edit's
// overlay that could not read the project's manifest file leaves its owed
// marker (OweManifestNotice), and the next prompt says it once
// (FileNoticeManifest). The post's line that no reader here claims a kind
// of file keeps one marker per kind and one for the kinds past its limit
// (kindnotice.go: IsKindNoticeFile, FileNoticeOtherKinds), and its line that
// the index holds no records of a language one marker per language
// (IsUnindexedNoticeFile).

// isNotice reports whether name is one of the session's notice markers.
func isNotice(name string) bool {
	switch name {
	case FileNoticeChannel, FileNoticeHoldout, FileNoticeCoverage, FileNoticeCoverageClear, FileNoticeIndexCap,
		FileNoticeManifestOwed, FileNoticeManifestReadFailed, FileNoticeManifest, FileNoticeOtherKinds:
		return true
	}
	return IsKindNoticeFile(name) || IsUnindexedNoticeFile(name)
}

// TakeNotice creates the session's marker name, one of the notice markers
// (isNotice), and reports whether this call created it: true once per
// session and marker, whoever races for it, and false after. Any other name,
// and any error, is false, which prints no notice.
func (s *Store) TakeNotice(sessionID, name string) bool {
	if !isNotice(name) || s.opened() != nil {
		return false
	}
	return s.createMarker(sessionID, name)
}

// OweManifestNotice leaves the session's owed manifest notice: an edit's
// overlay could not read the project's manifest file, readFailed when the
// read failed and not because the file is not a regular one. The class
// marker is made before the owed one, so a prompt that sees the owed marker
// sees its class too; the line names the failed read when any refusal of the
// session was one. Each marker is an empty file made once; an error makes
// none and leaves the session without the line. The overlay writer calls it
// with its own store handle and session (internal/hook overlayCall), never an
// os call of its own.
func (s *Store) OweManifestNotice(sessionID string, readFailed bool) {
	if s == nil || s.opened() != nil {
		return
	}
	if readFailed {
		s.createMarker(sessionID, FileNoticeManifestReadFailed)
	}
	s.createMarker(sessionID, FileNoticeManifestOwed)
}

// createMarker creates the session's empty marker name with an exclusive
// create and reports whether this call created it.
func (s *Store) createMarker(sessionID, name string) bool {
	f, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

// NoticeTaken reports whether the session's marker name is present, for a
// caller that must know a notice was said before it says anything: the
// Stop's cleared line follows its coverage notice and is never said without
// one. Any other name, and any error, is false.
func (s *Store) NoticeTaken(sessionID, name string) bool {
	if !isNotice(name) || s.opened() != nil {
		return false
	}
	_, err := os.Lstat(filepath.Join(s.RunDir(sessionID), name))
	return err == nil
}

// ReturnNotice removes the session's marker name, for a call that took it
// and then could not write the line that records the notice, so a later call
// prints it.
func (s *Store) ReturnNotice(sessionID, name string) {
	if isNotice(name) {
		_ = os.Remove(filepath.Join(s.RunDir(sessionID), name))
	}
}
