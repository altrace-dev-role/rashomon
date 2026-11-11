package graph

import (
	"slices"
	"strconv"
)

// The node kinds a session writes (ruling 15, W-M2.11). A lesson keeps its
// knowledge kind; its learned-from edges are EvidencedBy.
const (
	KindSession = "session"
	KindPrompt  = "prompt"
	KindCall    = "call"
	KindClaim   = "claim"
	KindNote    = "note"
	KindVerdict = "verdict"
)

// SessionKinds is the six kinds, a fresh slice each call.
func SessionKinds() []string {
	return []string{KindSession, KindPrompt, KindCall, KindClaim, KindNote, KindVerdict}
}

// IsSessionKind reports whether k is one of SessionKinds. A reader that
// scans every node (engine scan) skips these: a session's words are not
// the code's corpus (non-interference, ruling F2).
func IsSessionKind(k string) bool { return slices.Contains(SessionKinds(), k) }

// Session ids are local ids in the code index's own space, so an edge to
// file:<path> or a sym: id lands on the index's node when both files load
// into one Store.

// SessionNodeID is session:<sid>.
func SessionNodeID(sid string) ID { return ID("session:" + sid) }

// PromptNodeID is prompt:<sid>:<n>, n the prompt's order in the session.
func PromptNodeID(sid string, n int) ID { return ID("prompt:" + sid + ":" + strconv.Itoa(n)) }

// CallNodeID is call:<sid>:<tool_use_id>.
func CallNodeID(sid, toolUseID string) ID { return ID("call:" + sid + ":" + toolUseID) }

// ClaimNodeID is claim:<sid>:<n>.
func ClaimNodeID(sid string, n int) ID { return ID("claim:" + sid + ":" + strconv.Itoa(n)) }

// NoteNodeID is note:<sid>:<n>.
func NoteNodeID(sid string, n int) ID { return ID("note:" + sid + ":" + strconv.Itoa(n)) }

// VerdictNodeID is verdict:<sid>:<n>.
func VerdictNodeID(sid string, n int) ID { return ID("verdict:" + sid + ":" + strconv.Itoa(n)) }
