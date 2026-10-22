package recap

import (
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ForCodex is the digest recap speaks from under Codex: d, less the one
// coverage reason every Codex turn carries.
//
// A Codex run's coverage is unverified with hook_entry_unresolved on every
// record, by design: this build cannot read whether Codex trusts our entries
// (KARTHIK_REVIEW.md, item 4). Spoken, it would put the line on every turn,
// and a line on every turn is one nobody reads -- the failure the line exists
// to avoid. So it is set aside here, and here only: the report and the digest
// still carry it. Any other reason still speaks, and a turn whose only reason
// was this one is no longer unknown for want of coverage. Decided by Rishi,
// for Karthik's review.
func ForCodex(d *digest.Digest) *digest.Digest {
	c := *d
	c.Coverage.Reasons = make([]string, 0, len(d.Coverage.Reasons))
	for _, r := range d.Coverage.Reasons {
		if r != store.ReasonHookEntryUnresolved {
			c.Coverage.Reasons = append(c.Coverage.Reasons, r)
		}
	}
	if len(c.Coverage.Reasons) == 0 {
		c.Coverage.State = store.StateVerified
		c.Unknown = false
	}
	return &c
}
