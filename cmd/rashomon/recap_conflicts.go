package main

import (
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// conflictsSeen is the conflicts the session was shown, for the Stop's
// StopInput.Conflicts (W-M2.4). Verify half: no production caller fills it,
// so it returns nil and the Stop is unchanged; the hook seam (the next card)
// replaces it with the PostToolUse record's reading, and the tests of this
// half set it to drive the Stop end to end.
var conflictsSeen = func(run *store.Run, seq int64) []verify.ConflictSeen { return nil }
