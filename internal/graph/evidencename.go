package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// StoreDir is the directory under the store root that holds the graph's
// own files: the session log and the hook path's evidence files.
const StoreDir = "graph"

const (
	evidenceExt    = ".txt"
	evidenceStamp  = "20060102T150405.000"
	sessionLogExt  = ".jsonl"
	sessionLogSum  = 8
	sessionLogName = 2 * sessionLogSum
)

// EvidenceFileName is the name of the evidence file for a prompt:
// <session>-<prompt>.txt, each id passed through the store's segment rule
// so a hook id holding "/../.." stays one name under StoreDir, and the UTC
// stamp in place of a prompt id when there is none. A plain UUID session
// and prompt keep their ids byte for byte. A segment that begins with a
// dot or a dash is digested too, so the name is never a dotfile and
// IsEvidenceFileName can read it back.
func EvidenceFileName(sessionID, promptID string, now time.Time) string {
	last := now.UTC().Format(evidenceStamp)
	if promptID != "" {
		last = nameSegment(promptID)
	}
	return nameSegment(sessionID) + "-" + last + evidenceExt
}

// nameSegment is store.Segment, forced to the digest form for an id that
// would begin the name with a dot or a dash.
func nameSegment(id string) string {
	s := store.Segment(id)
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "-") {
		return store.Segment("\x00" + id)
	}
	return s
}

// IsEvidenceFileName is whether name is a value EvidenceFileName can
// return: two segments joined by a dash and ".txt", the second a segment or
// the stamp, neither beginning with a dot or a dash. It is the purge side
// of the same rule, and no tighter than the writer allows.
func IsEvidenceFileName(name string) bool {
	stem, ok := strings.CutSuffix(name, evidenceExt)
	if !ok {
		return false
	}
	for i := 1; i < len(stem)-1; i++ {
		if stem[i] != '-' {
			continue
		}
		if isNameSegment(stem[:i]) && (isNameSegment(stem[i+1:]) || isStamp(stem[i+1:])) {
			return true
		}
	}
	return false
}

func isNameSegment(s string) bool {
	return store.IsSegment(s) && !strings.HasPrefix(s, ".") && !strings.HasPrefix(s, "-")
}

func isStamp(s string) bool {
	_, err := time.Parse(evidenceStamp, s)
	return err == nil && len(s) == len(evidenceStamp)
}

// sessionLogFile is the session log's file name for repository rid:
// 16 hex of sha256(rid) and ".jsonl".
func sessionLogFile(rid string) string {
	sum := sha256.Sum256([]byte(rid))
	return hex.EncodeToString(sum[:sessionLogSum]) + sessionLogExt
}

// IsSessionLogName is whether name is a value sessionLogFile can return:
// 16 lowercase hex and ".jsonl".
func IsSessionLogName(name string) bool {
	stem, ok := strings.CutSuffix(name, sessionLogExt)
	if !ok || len(stem) != sessionLogName || stem != strings.ToLower(stem) {
		return false
	}
	_, err := hex.DecodeString(stem)
	return err == nil
}
