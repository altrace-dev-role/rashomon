package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// clearFiles is the /clear rendezvous's inventory (store.MeetClear): in
// clear/<digest>, one folder per working directory named by the SHA-256
// digest of its path, an empty arm per side of a /clear, the link that pairs
// them and the link's temporary. purge reads them back with
// store.IsClearDirName and store.IsClearName.
func clearFiles() []FileRule {
	const (
		base  = store.DirClear + "/*"
		never = "the working directory's path, or anything about a call"
		swept = "the next /clear in that folder that finds it more than an hour old; purge"
	)
	return []FileRule{
		{
			Base:       base,
			Pattern:    "end-*",
			Mode:       "0600",
			Purpose:    "the old session's side of a /clear, left by its SessionEnd so the new session's SessionStart in the same working directory can pair with it",
			Holds:      "nothing: it is empty; its name holds the old session's id",
			NeverHolds: never,
			RemovedBy:  "the side that writes the pair's link; " + swept,
			Owner:      "store",
		},
		{
			Base:       base,
			Pattern:    "start-*",
			Mode:       "0600",
			Purpose:    "the new session's side of a /clear, left by its SessionStart so the old session's SessionEnd in the same working directory can pair with it",
			Holds:      "nothing: it is empty; its name holds the new session's id",
			NeverHolds: never,
			RemovedBy:  "the side that writes the pair's link; " + swept,
			Owner:      "store",
		},
		{
			Base:       base,
			Pattern:    "link-*",
			Mode:       "0600",
			Purpose:    "a /clear's pair: the new session, named by the file, continues the old one",
			Holds:      "the old session's id; its name holds the new session's id",
			NeverHolds: never,
			RemovedBy:  swept,
			Owner:      "store",
		},
		{
			Base:       base,
			Pattern:    ".link-*.tmp-*",
			Mode:       "0600",
			Purpose:    "a link's temporary; present only between its write and its rename to link-*, or after a crash in between",
			Holds:      "what the link is about to hold",
			NeverHolds: never,
			RemovedBy:  "the write that made it (renamed to link-*); " + swept,
			Owner:      "store",
		},
	}
}
