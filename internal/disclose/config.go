package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// configFiles is the inventory's rows for the settings file `rashomon summary
// on|off` writes (card REPORT1a, FUN-16), and the temporary file its write
// leaves. Like firedFiles, it sits apart from defaultFiles for Law 10.
func configFiles() []FileRule {
	return []FileRule{
		{
			Base:       "",
			Pattern:    store.FileConfig,
			Mode:       "0600",
			Purpose:    "the person's switches; today one, whether the end-of-turn line prints its value line (`rashomon summary on|off`); absent, every switch is at its default and the summary is on",
			Holds:      "the switch's name and its on or off value",
			NeverHolds: "a prompt, a session id, a path, or anything about a call",
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
