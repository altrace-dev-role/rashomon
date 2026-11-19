package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

// The unclaimed kind's notice markers (card LANG-REG1): the first time a
// session edits a file of a kind no reader here claims, the post hook says so
// once to the person, naming the kind, and the marker is how a later edit of
// that kind knows the session has had the line. Like the other notice
// markers, each is an empty file written without a sync: a crash that loses
// one says the line once more, and nothing else. Its name is
// FileNoticeKindPrefix and 16 lower-case hex digits, the start of the sha256
// of the kind's key (lang.KindKey: the extension, or the name of a file keyed
// by its name), so no key is ever a file name here. The digest keeps the name
// safe, not secret: the kinds are few and easy to guess.
//
// A session keeps at most kindNoticeLimit of them. A kind past the limit
// takes the one FileNoticeOtherKinds marker instead, whose line names no
// kind, so the limit is said once and never silently; the hook sends a key
// it will not print to that marker too (TakeNotice), and a later kind under
// the limit still takes its own.
const FileNoticeKindPrefix = "notice-kind-"

// kindDigestLen is the hex digits after FileNoticeKindPrefix.
const kindDigestLen = 16

// kindNoticeLimit is the most kind markers a session keeps.
const kindNoticeLimit = 16

// The unindexed language's notice markers: the first time a session edits a
// file whose reader is registered indexed (lang.Registry.RegisterIndexed)
// under an index that holds no records of the reader's language, the post
// hook says so once to the person, naming the language. Each is an empty
// file like the kind markers, named FileNoticeUnindexedPrefix and the same
// 16 hex digits of the language's name. The languages are the registry's,
// never a file's, so a session keeps at most one per registered language
// and no limit is needed; the family is apart from the kind markers, so it
// is never counted among them.
const FileNoticeUnindexedPrefix = "notice-unindexed-"

// KindNoticeName returns the marker name for the kind key: the prefix and the
// first 16 lower-case hex digits of the key's sha256.
func KindNoticeName(key string) string { return digestName(FileNoticeKindPrefix, key) }

// IsKindNoticeFile reports whether name is one of the kind markers' file
// names: the prefix and 16 lower-case hex digits.
func IsKindNoticeFile(name string) bool { return isDigestName(FileNoticeKindPrefix, name) }

// UnindexedNoticeName returns the marker name for the language lang: the
// prefix and the first 16 lower-case hex digits of its sha256.
func UnindexedNoticeName(lang string) string { return digestName(FileNoticeUnindexedPrefix, lang) }

// IsUnindexedNoticeFile reports whether name is one of the unindexed
// language markers' file names: the prefix and 16 lower-case hex digits.
func IsUnindexedNoticeFile(name string) bool { return isDigestName(FileNoticeUnindexedPrefix, name) }

// digestName is prefix and the first 16 lower-case hex digits of key's
// sha256.
func digestName(prefix, key string) string {
	sum := sha256.Sum256([]byte(key))
	return prefix + hex.EncodeToString(sum[:kindDigestLen/2])
}

// isDigestName reports whether name is prefix and 16 lower-case hex digits.
func isDigestName(prefix, name string) bool {
	d, ok := strings.CutPrefix(name, prefix)
	if !ok || len(d) != kindDigestLen {
		return false
	}
	for i := 0; i < len(d); i++ {
		if c := d[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// TakeKindNotice takes the session's notice for the kind key and returns the
// marker it took: the kind's own (KindNoticeName), FileNoticeOtherKinds when
// the session already keeps kindNoticeLimit kinds, or "" when no line is
// owed: the kind's marker is there already, the session is past the limit
// and FileNoticeOtherKinds is there too, or a read or create failed. The
// caller says the kind's line for the kind's marker and the line naming no
// kind for FileNoticeOtherKinds, and gives the marker back (ReturnNotice)
// when it could not say it.
//
// The kinds are counted again after the create, and a take that made them
// more than the limit removes its own marker and takes FileNoticeOtherKinds,
// so posts that race never leave more than the limit: two that pass it at
// once both fall back, and the line naming no kind is said once. A count
// that cannot be read takes nothing, a missed line and never one more than
// the limit.
func (s *Store) TakeKindNotice(sessionID, key string) string {
	if s == nil || s.opened() != nil {
		return ""
	}
	name := KindNoticeName(key)
	if s.NoticeTaken(sessionID, name) {
		return ""
	}
	n, ok := s.kindNotices(sessionID)
	if !ok {
		return ""
	}
	if n >= kindNoticeLimit {
		return s.takeOtherKinds(sessionID)
	}
	if !s.createMarker(sessionID, name) {
		return ""
	}
	if n, ok = s.kindNotices(sessionID); !ok || n > kindNoticeLimit {
		s.ReturnNotice(sessionID, name)
		if !ok {
			return ""
		}
		return s.takeOtherKinds(sessionID)
	}
	return name
}

// takeOtherKinds takes FileNoticeOtherKinds and returns it, or "" when an
// earlier call took it or the create failed.
func (s *Store) takeOtherKinds(sessionID string) string {
	if s.TakeNotice(sessionID, FileNoticeOtherKinds) {
		return FileNoticeOtherKinds
	}
	return ""
}

// kindNotices counts the session's kind markers, and reports false when its
// run directory could not be read.
func (s *Store) kindNotices(sessionID string) (int, bool) {
	ents, err := os.ReadDir(s.RunDir(sessionID))
	if err != nil {
		return 0, false
	}
	n := 0
	for _, e := range ents {
		if IsKindNoticeFile(e.Name()) {
			n++
		}
	}
	return n, true
}
