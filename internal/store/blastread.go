package store

import (
	"os"
	"path/filepath"
	"strings"
)

// The blast-radius read's markers (card ARCH-INLINE1, A2's read gate): one
// per package and platform-graph log version a session has read the callers
// of, so the log is read about once per package each time pgraph syncs, not
// once per edit. Like the notice markers, a marker is an empty file written
// without a sync: a crash that loses one costs one more read, and nothing
// else. Its name is FileBlastReadPrefix and 16 hex digits, a digest the hook
// takes of the package and the log's sha256, so no package path is ever a
// file name here.
const FileBlastReadPrefix = "blast-read-"

// blastDigestLen is the hex digits after FileBlastReadPrefix.
const blastDigestLen = 16

// IsBlastReadFile reports whether name is one of the markers' file names:
// the prefix and 16 lower-case hex digits.
func IsBlastReadFile(name string) bool {
	d, ok := strings.CutPrefix(name, FileBlastReadPrefix)
	if !ok || len(d) != blastDigestLen {
		return false
	}
	for i := 0; i < len(d); i++ {
		if c := d[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// TakeBlastRead creates the session's marker name and reports whether this
// call created it: true once per session and name, whoever races for it, and
// false after. A name IsBlastReadFile refuses, and any error, is false, which
// reads nothing: a missed note, never a second one.
func (s *Store) TakeBlastRead(sessionID, name string) bool {
	if !IsBlastReadFile(name) || s.opened() != nil {
		return false
	}
	f, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

// ReturnBlastRead removes the session's marker name, for a call that took it
// and then did not deliver its note (a correction outranked it, or the
// prompt's budget was spent), so the package's next edit reads again.
func (s *Store) ReturnBlastRead(sessionID, name string) {
	if IsBlastReadFile(name) {
		_ = os.Remove(filepath.Join(s.RunDir(sessionID), name))
	}
}
