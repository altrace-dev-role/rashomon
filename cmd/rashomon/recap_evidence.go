package main

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// checkEvidence is the Stop's channel-evidence step (card CHANNEL-EVIDENCE1):
// each delivered note of the session that no Stop has judged is looked for in
// Claude Code's transcript, and the outcomes are recorded (steer.CheckEvidence).
// It is the one transcript read a hook makes. recapTurn defers it past the
// checkpoint's flush and the line's print, so the record is durable and the
// line is out before it reads anything, and a transcript that is slow, huge or
// gone costs the Stop nothing it printed: steer.DefaultScanLimits bounds the
// read to the file's last 16 MiB and one second. A Stop with no note to judge
// opens no file. Its panic and its errors are dropped, as the rest of the
// entry's are (H-93): evidence is bookkeeping, and the entry exits 0.
func checkEvidence(st *store.Store, sessionID, transcript string) {
	if sessionID == "" || transcript == "" {
		return
	}
	_ = safe.Guard(func() error {
		now := time.Now()
		steer.CheckEvidence(st, sessionID, transcript, now, steer.DefaultScanLimits(now))
		return nil
	})
}
