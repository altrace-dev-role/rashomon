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
		Purpose: "one per Go package and platform-graph log, or per function or method and repository graph, " +
			"the session read the callers of after an edit, so the log is read once per package each time pgraph syncs " +
			"and the repository graph once per function or method for each commit and record count it is written with",
		Holds:      "nothing; its name is a keyed digest of the package and the log, or of the function or method and the repository graph's commit and record count",
		NeverHolds: "anything",
		RemovedBy:  "size-cap eviction, with its run; purge",
		Owner:      "store",
	}}
}
