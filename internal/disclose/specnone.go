package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// specNoneFiles is the inventory's row for the no-spec marker (card
// SPEC-RULES1, D210), apart from defaultFiles for Law 10.
func specNoneFiles(runSegment string) []FileRule {
	return []FileRule{
		{
			Base:       runSegment,
			Pattern:    store.FileSpecNone,
			Mode:       "0600",
			Purpose:    "a hint that the turn's prompt preflight found no spec, so the turn's tool calls do not look for one again",
			Holds:      "the turn's prompt id and a keyed digest of the directory the preflight looked from, and the word none; no path",
			NeverHolds: "a path, a spec, a command, or anything a spec says",
			RemovedBy:  "the next prompt's preflight; a call that names rashomon's own directory; size-cap eviction; purge",
			Owner:      "store",
		},
	}
}
