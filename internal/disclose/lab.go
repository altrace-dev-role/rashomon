package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// labFiles is the inventory's rows for the lab marker `rashomon measure
// --prepare` writes (card MEASURE1) and the temporary file its write leaves.
// Like indexFiles, they sit apart from defaultFiles for Law 10.
func labFiles() []FileRule {
	return []FileRule{
		{
			Base:       "",
			Pattern:    store.FileLab,
			Mode:       "0600",
			Purpose:    "marks the store as a measurement lab, the only kind of store whose hooks honour RASHOMON_STEER_ARM; absent on every store `measure --prepare` was not run on",
			Holds:      "one fixed sentence saying so; only the file's presence is read",
			NeverHolds: "a session id, a path, or anything about a call",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileLab + ".tmp-*",
			Mode:       "0600",
			Purpose:    "lab's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "the same fixed sentence",
			NeverHolds: "a session id, a path, or anything about a call",
			RemovedBy:  "the write that made it (renamed over lab); purge",
			Owner:      "store",
		},
	}
}
