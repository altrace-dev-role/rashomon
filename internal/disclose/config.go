package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// configFiles is the inventory's rows for the settings file `rashomon summary
// on|off` and `rashomon steer off|on` write (cards REPORT1a, FUN-16 and
// STEER-CLI1), the lock its writers take, and the temporary file its write
// leaves. Like firedFiles, it sits apart from defaultFiles for Law 10.
func configFiles() []FileRule {
	return []FileRule{
		{
			Base:       "",
			Pattern:    store.FileConfig,
			Mode:       "0600",
			Purpose:    "the person's switches: whether the end-of-turn line prints its value line (`rashomon summary on|off`) and whether the notes print (`rashomon steer off|on`), each for you or for one project; absent, every switch is at its default and both are on",
			Holds:      "each switch's name and its on or off value; for a project's switch, the project's directory, as the person's working directory named it",
			NeverHolds: "a prompt, a session id, or anything about a call",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			// UpdateConfig locks this sidecar, never config.json, which is
			// replaced by rename under the lock.
			Base:       "",
			Pattern:    store.FileConfigLock,
			Mode:       "0600",
			Purpose:    "the lock config.json's writer takes; its content is never read",
			Holds:      "nothing",
			NeverHolds: "anything",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileConfig + ".tmp-*",
			Mode:       "0600",
			Purpose:    "config.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what config.json is about to hold",
			NeverHolds: "a prompt, a session id, a path, or anything about a call",
			RemovedBy:  "the write that made it (renamed over config.json); purge",
			Owner:      "store",
		},
	}
}
