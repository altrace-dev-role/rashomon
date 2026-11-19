package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClearSide is which hook of a /clear is calling: the old session's
// SessionEnd or the new session's SessionStart.
type ClearSide uint8

const (
	ClearEnd ClearSide = iota + 1
	ClearStart
)

// ClearLink is a /clear's pair: New continues Old. Wrote is set for the one
// call that created the link; only that call records the continuation.
type ClearLink struct {
	Old, New string
	Wrote    bool
}

const (
	// ClearWindow is how far apart in time the two arms of one /clear may
	// be: two of the clear hooks' timeouts in plugin/hooks/hooks.json
	// (TestClearWindow_CoversTheClearHooksTimeout).
	ClearWindow = 10 * time.Second
	// ClearKeep is the age past which any visit removes an arm, a link or a
	// link's temporary. It sweeps; it does not pair.
	ClearKeep = time.Hour
)

// Entry names in a rendezvous directory: a kind and a session id. No name
// holds the cwd; the directory is named by a digest of it.
const (
	clearEndArm   = "end-"
	clearStartArm = "start-"
	clearLinkName = "link-"
)

// meetClear is one side of the /clear rendezvous in dir. The two hooks of a
// /clear share no id (SessionEnd carries the old session's, SessionStart the
// new one's) and the hooks docs do not order them. So each side leaves an
// arm, <kind>-<its id>, then lists dir, and links only on an unambiguous
// listing: its own arm is the one fresh arm of its kind, and one fresh arm of
// the other kind stands, of another session. A second fresh arm of its own
// kind means two /clears overlap in this cwd, where any pairing could be the
// wrong one, and a wrong link is worse than none; counting the other kind
// alone would miss that.
//
// The link is link-<new id> holding the old id, made by createExclusive, so
// a new session is linked at most once and of two racing sides exactly one
// writes (Wrote). A link already there is never rewritten: it confirms this
// pair when it names the same old session and refuses otherwise. The writer
// removes both arms, so a matched pair does not block the next /clear.
//
// An arm's time is its file's modification time as the filesystem set it;
// now is only compared with it. Every visit sweeps its own kinds of entries
// older than ClearKeep.
func meetClear(dir string, side ClearSide, self string, now time.Time) (ClearLink, bool, error) {
	if !isPlainSegment(self) {
		return ClearLink{}, false, nil
	}
	var mine, theirs string
	switch side {
	case ClearEnd:
		mine, theirs = clearEndArm, clearStartArm
	case ClearStart:
		mine, theirs = clearStartArm, clearEndArm
	default:
		return ClearLink{}, false, nil
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return ClearLink{}, false, err
	}
	arm := filepath.Join(dir, mine+self)
	f, err := os.OpenFile(arm, os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return ClearLink{}, false, err
	}
	if err := f.Close(); err != nil {
		return ClearLink{}, false, err
	}
	t, err := tallyClear(dir, mine+self, mine, theirs, now)
	if err != nil {
		return ClearLink{}, false, err
	}
	if !t.self || t.same != 1 || t.other != 1 || t.peer == self {
		return ClearLink{}, false, nil
	}
	l := ClearLink{Old: self, New: t.peer}
	if side == ClearStart {
		l = ClearLink{Old: t.peer, New: self}
	}
	link := filepath.Join(dir, clearLinkName+l.New)
	err = createExclusive(link, []byte(l.Old))
	if errors.Is(err, fs.ErrExist) {
		b, err := os.ReadFile(link)
		if err != nil {
			return ClearLink{}, false, err
		}
		if string(b) != l.Old {
			return ClearLink{}, false, nil
		}
		return l, true, nil
	}
	if err != nil {
		return ClearLink{}, false, err
	}
	os.Remove(arm)                               //nolint:errcheck // a left arm ages out of the window
	os.Remove(filepath.Join(dir, theirs+t.peer)) //nolint:errcheck // a left arm ages out of the window
	l.Wrote = true
	return l, true, nil
}

// clearTally is one listing of a rendezvous directory, counted for a caller
// whose arm kind is mine.
type clearTally struct {
	self        bool   // the caller's own arm is fresh
	same, other int    // fresh arms of the caller's kind and of the other
	peer        string // the other kind's session id when other is 1
}

// tallyClear lists dir once: it removes entries of meetClear's naming older
// than ClearKeep and counts the arms within ClearWindow of now. Names not of
// meetClear's making are left alone.
func tallyClear(dir, ownArm, mine, theirs string, now time.Time) (clearTally, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return clearTally{}, err
	}
	var t clearTally
	for _, e := range entries {
		name := e.Name()
		if !IsClearName(name) {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed by another visit since the listing
		}
		if err != nil {
			return clearTally{}, err
		}
		age := now.Sub(info.ModTime()).Abs()
		if age > ClearKeep {
			os.Remove(filepath.Join(dir, name)) //nolint:errcheck // a sweep; the next visit retries
			continue
		}
		if age > ClearWindow {
			continue
		}
		switch {
		case strings.HasPrefix(name, mine):
			t.same++
			t.self = t.self || name == ownArm
		case strings.HasPrefix(name, theirs):
			t.other++
			t.peer = strings.TrimPrefix(name, theirs)
		}
	}
	return t, nil
}

// IsClearName reports whether name is one meetClear makes: an arm, a link,
// or createExclusive's temporary for a link. A visit sweeps only such names,
// and purge owns only such files in a rendezvous folder.
func IsClearName(name string) bool {
	for _, p := range []string{clearEndArm, clearStartArm, clearLinkName} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return strings.HasPrefix(name, "."+clearLinkName) && strings.Contains(name, ".tmp-")
}
