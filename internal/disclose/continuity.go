package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// continuityFiles is the inventory's row for the continuity briefing's
// resume marker (card COMPACT-SLOT1, D-c). The briefing's line is
// steer.ndjson's, and the transcript it quotes is read in memory, never
// stored, so the marker is the one file the briefing adds.
func continuityFiles() []FileRule {
	return []FileRule{
		{
			Base:       store.DirRuns + "/<session>",
			Pattern:    store.FileBriefingArmed,
			Mode:       "0600",
			Purpose:    "says a resumed session's next prompt is to give the briefing its last compaction did not",
			Holds:      "nothing; present after a resumed session's start until its next prompt",
			NeverHolds: "anything",
			RemovedBy:  "the session's next prompt; size-cap eviction, with its run; purge",
			Owner:      "store",
		},
	}
}
