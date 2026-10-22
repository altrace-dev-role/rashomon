package report

import "github.com/altrace-dev-role/rashomon/internal/store"

// DroppedReasons is the coverage reason for each kind of lone terminal among
// ids (store.Run.Dropped, or a turn's share of it): lock_timeout for one
// whose own reason is lock_timeout, which is the terminal a hook writes to
// the spill file when the lock refused its declaration, and
// declaration_missing for any other. The report and the turn digest both
// take their reasons from here, so the two cannot disagree on what a lone
// terminal means.
func DroppedReasons(run *store.Run, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	lone := make(map[string]bool, len(ids))
	for _, id := range ids {
		lone[id] = true
	}
	locked, missing := false, false
	for _, t := range run.Terminals {
		if !lone[t.ToolUseID] || t.FlushFailed() {
			continue
		}
		if t.Reason != nil && *t.Reason == store.ReasonLockTimeout {
			locked = true
		} else {
			missing = true
		}
	}
	var reasons []string
	if locked {
		reasons = append(reasons, store.ReasonLockTimeout)
	}
	if missing {
		reasons = append(reasons, ReasonDeclarationMissing)
	}
	return reasons
}
