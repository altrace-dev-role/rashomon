package store

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The no-spec marker (card SPEC-RULES1, D210). The UserPromptSubmit
// preflight discovers the spec when the prompt names a path or an
// identifier; when it finds none it notes that here, for the turn and the
// directory it discovered from, so the rules step of the turn's posts skips a
// second discovery of the same nothing. The marker is a hint that only
// narrows a discovery to none for a turn: it is never a spec, never a trust
// and never read by a Stop. It is written without a sync and read as
// untrusted bytes: anything missing, unreadable or malformed is no marker, so
// the reader runs the real discovery. It holds no path.

// specNoneMaxBytes bounds a marker read: a turn id and a 64-digit digest fit
// well inside it.
const specNoneMaxBytes = 512

// NoteNoSpec records that a discovery for the session's turn promptID, from
// the directory whose keyed digest is digest, found no spec. An empty id, or
// one with a newline, removes the marker instead of writing one.
func (s *Store) NoteNoSpec(sessionID, promptID, digest string) error {
	if err := s.opened(); err != nil {
		return err
	}
	path := filepath.Join(s.RunDir(sessionID), FileSpecNone)
	if promptID == "" || strings.ContainsAny(promptID+digest, "\n\r") {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte("none\n"+promptID+"\n"+digest+"\n"), fileMode)
}

// NoSpecNoted reports whether the session holds a marker for exactly this
// turn and digest. Every other state, a missing file, a read error, a
// marker of another turn or directory, a malformed one, is false.
func (s *Store) NoSpecNoted(sessionID, promptID, digest string) bool {
	if promptID == "" || s.opened() != nil {
		return false
	}
	f, err := os.Open(s.RunDir(sessionID) + string(filepath.Separator) + FileSpecNone)
	if err != nil {
		return false
	}
	// One byte over the cap tells an oversize marker from a full one; the
	// buffer stays on the stack where ReadFile's would not.
	var buf [specNoneMaxBytes + 1]byte
	n, _ := io.ReadFull(f, buf[:])
	_ = f.Close()
	if n > specNoneMaxBytes {
		return false
	}
	b := buf[:n]
	b, ok := bytes.CutPrefix(b, []byte("none\n"))
	if !ok {
		return false
	}
	turn, b, ok := bytes.Cut(b, []byte("\n"))
	if !ok || string(turn) != promptID {
		return false
	}
	dig, b, ok := bytes.Cut(b, []byte("\n"))
	return ok && string(dig) == digest && len(b) == 0
}

// ClearNoSpec removes the session's marker: a call that named
// .claude/rashomon may have made a spec.
func (s *Store) ClearNoSpec(sessionID string) {
	if s.opened() == nil {
		_ = os.Remove(filepath.Join(s.RunDir(sessionID), FileSpecNone))
	}
}
