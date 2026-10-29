package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// bodiesFiles is the inventory's rows for the transcript copies and their
// switch (card 4.1, PF-RC14), and the temporary files their writes leave.
// Like indexFiles, they sit apart from defaultFiles for Law 10.
func bodiesFiles() []FileRule {
	return []FileRule{
		{
			Base:       "",
			Pattern:    store.FileBodies,
			Mode:       "0600",
			Purpose:    "the transcript-copy switch: the projects whose sessions keep a copy of their transcript, and the copies' total cap; absent, no copy is taken",
			Holds:      "each such project's directory in the clear, and the cap in bytes",
			NeverHolds: "a transcript, a session id, or anything about a call",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileBodies + ".tmp-*",
			Mode:       "0600",
			Purpose:    "bodies.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what bodies.json is about to hold",
			NeverHolds: "a transcript, a session id, or anything about a call",
			RemovedBy:  "the write that made it (renamed over bodies.json); purge",
			Owner:      "store",
		},
		{
			Base:       store.DirBodies,
			Pattern:    "*/*.jsonl",
			Mode:       "0600",
			Purpose:    "a byte copy of one session's transcript, taken at SessionEnd for a project whose copies are on, and read by CLI commands only",
			Holds:      "the transcript as Claude Code wrote it: the prompts, the agent's messages, its tool calls and their results",
			NeverHolds: "anything the transcript does not hold; nothing is added to it",
			RemovedBy:  "eviction of the oldest copies at the switch's cap; purge",
			Owner:      "store",
		},
		{
			Base:       store.DirBodies,
			Pattern:    "*/" + store.BodiesTempPrefix + "*",
			Mode:       "0600",
			Purpose:    "a transcript copy in flight; present only until it is renamed over its session's copy, or after a crash in between",
			Holds:      "the part of the transcript copied so far",
			NeverHolds: "anything the transcript does not hold",
			RemovedBy:  "the copy that made it (renamed over <session>.jsonl); eviction at the cap; purge",
			Owner:      "store",
		},
	}
}
