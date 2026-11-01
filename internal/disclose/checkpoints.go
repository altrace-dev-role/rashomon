package disclose

import (
	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// checkpointFiles is the inventory's rows for the project's last two
// checkpoints, which the Stop that cuts one notes in a file beside the
// project index (verify.CheckpointsPath, INDEX-POINTER-FILE1), and its
// replacement. They sit in a file of their own for Law 10.
func checkpointFiles() []FileRule {
	const base = verify.DirName + "/" + verify.CheckpointsDir
	return []FileRule{
		{
			Base:       base,
			Pattern:    baseline.FilePattern,
			Mode:       "0600",
			Purpose:    "one project's last two checkpoints by session, so a new session's brief names the previous session's in one small read",
			Holds:      "the last and prior checkpoints (last, prior), each the session id, the point seq and when it was recorded, written at every Stop that cuts one",
			NeverHolds: "the project's directory, a command, or anything else about a call",
			RemovedBy:  "the next Stop's write, which replaces it; purge",
			Owner:      "verify",
		},
		{
			Base:       base,
			Pattern:    "." + baseline.FilePattern + ".tmp-*",
			Mode:       "0600",
			Purpose:    "a checkpoint file's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what the checkpoint file is about to hold",
			NeverHolds: "the project's directory, a command, or anything else about a call",
			RemovedBy:  "the write that made it (renamed over the checkpoint file); purge",
			Owner:      "verify",
		},
	}
}
