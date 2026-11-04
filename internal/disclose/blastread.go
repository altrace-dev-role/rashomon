package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// blastReadFiles is the inventory's row for the blast-radius read's markers
// (store.TakeBlastRead, card ARCH-INLINE1), apart from defaultFiles for Law
// 10 as firedFiles is.
func blastReadFiles() []FileRule {
	return []FileRule{{
		Base:    store.DirRuns + "/<session>",
		Pattern: store.FileBlastReadPrefix + "*",
		Mode:    "0600",
		Purpose: "one per Go package and platform-graph log the session read the callers of after an edit, " +
			"so the log is read once per package each time pgraph syncs",
		Holds:      "nothing; its name is a keyed digest of the package and the log",
		NeverHolds: "anything",
		RemovedBy:  "size-cap eviction, with its run; purge",
		Owner:      "store",
	}}
}
