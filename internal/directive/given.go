package directive

import (
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// Given reports whether lines, a session's steer lines, hold the continuity
// briefing under key for agent ("" for the main agent): a line delivered, or
// withheld by arm off, whose shadow decision gave it, as learn.Delivered
// reads a lesson's. One briefing per session, agent and compaction (D-b):
// its key is the boundary's uuid, so the next compaction briefs again. A line
// suppressed on its way was never given, so a later start at the same
// boundary tries again. Lines are located with jsonscan, never decoded
// (RES-5).
func Given(lines [][]byte, key, agent string) bool {
	var out [gkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, givenKeys(), out[:]) != nil || text(b, out[gkPattern]) != string(pattern.ContinuityBriefing) ||
			text(b, out[gkKey]) != key || text(b, out[gkAgent]) != agent {
			continue
		}
		if text(b, out[gkState]) == string(steer.LineStateDelivered) || text(b, out[gkSuppressedBy]) == string(steer.SuppressedByArmOff) {
			return true
		}
	}
	return false
}

// The keys Given reads a line by, in givenKeys' order.
const (
	gkAgent = iota
	gkKey
	gkPattern
	gkState
	gkSuppressedBy
	gkCount
)

// givenKeys is the one KeySet Given reads a steer line with.
func givenKeys() *jsonscan.KeySet {
	givenKeysOnce.Do(func() {
		givenKeysOnce.v = jsonscan.NewKeySet("agent_id", "key", "pattern", "state", "suppressed_by")
	})
	return givenKeysOnce.v
}

// givenKeysOnce holds what givenKeys returns, built on its first call and
// never at init (RES-2).
var givenKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}
