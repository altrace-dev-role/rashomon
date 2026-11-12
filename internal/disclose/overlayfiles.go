package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// overlayRunFiles is the inventory's rows for the two files the overlay
// journal's bound leaves in a run directory (store.ReserveOverlay, card
// W-M2.5c, 5c-2), apart from defaultFiles for Law 10 as blastReadFiles is.
func overlayRunFiles() []FileRule {
	const runSegment = store.DirRuns + "/<session>"
	return []FileRule{
		{
			Base:       runSegment,
			Pattern:    store.FileOverlayBytes,
			Mode:       "0600",
			Purpose:    "the running byte count of this session's overlay journal, checked against its 4 MiB bound",
			Holds:      "one decimal number",
			NeverHolds: "file contents, paths or anything else",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileOverlayFull,
			Mode:       "0600",
			Purpose:    "an empty marker, created once, when the overlay journal's bound refused a group",
			Holds:      "nothing; present once the bound refused a group",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
	}
}
