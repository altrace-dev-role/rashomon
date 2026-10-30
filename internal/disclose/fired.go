package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// firedFiles is the inventory's row for the hook entries' fired markers
// (STATUS-VERBS1), one file per event in each run directory. Like bodiesFiles,
// it sits apart from defaultFiles for Law 10.
func firedFiles() []FileRule {
	return []FileRule{{
		Base:       store.DirRuns + "/<session>",
		Pattern:    store.FileFiredPrefix + "*",
		Mode:       "0600",
		Purpose:    "one per hook event, the mark that its entry ran in the session; status counts the sessions each entry fired in from these",
		Holds:      "one Unix-millisecond timestamp, the event's first fire in the session",
		NeverHolds: "anything else",
		RemovedBy:  "size-cap eviction; purge",
		Owner:      "store",
	}}
}
