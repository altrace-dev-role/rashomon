package main

import (
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// conflictsSeen is the conflicts the session was shown, for the Stop's
// StopInput.Conflicts (W-M2.4). Verify half: no production caller fills it,
// so it returns nil and the Stop is unchanged; the hook seam (the next card)
// replaces it with the PostToolUse record's reading, and the tests of this
// half set it to drive the Stop end to end.
var conflictsSeen = func(run *store.Run, seq int64) []verify.ConflictSeen { return nil }

// stopLineWith is recap.LineWith over the Stop's rule and platform lines, and,
// when the Stop speaks and a conflict is unresolved, a second composition with
// the conflict lines added (W-M2.4). The conflict lines come last and take
// what the message leaves under Claude Code's cap, so they are composed
// against the message as the first pass left it (B2). With no conflict there
// is no second pass: the common Stop pays nothing. It is separate from
// recapTurn to keep that function within Law 10.
func stopLineWith(d *digest.Digest, stop stopTurn, fromPlugin bool) (string, bool) {
	lines := stop.rules() + recap.PlatformLines(stop.view, fromPlugin)
	line, wantSpeak := recap.LineWith(d, d.SessionID, fromPlugin, lines)
	if !wantSpeak || !stop.speaks {
		return line, wantSpeak
	}
	if cl := recap.ConflictLines(stop.view, fromPlugin, line); cl != "" {
		line, _ = recap.LineWith(d, d.SessionID, fromPlugin, lines+cl)
	}
	return line, wantSpeak
}
