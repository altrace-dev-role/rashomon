package disclose

import "github.com/altrace-dev-role/rashomon/internal/learn"

// learnFiles is the inventory's rows for the lesson ledger (card P6): the
// learner's key, each project's state files and manifest, and the temporary
// files their writes leave. Like bodiesFiles, they sit apart from
// defaultFiles for Law 10.
func learnFiles() []FileRule {
	project := func(name, purpose, holds, never string) FileRule {
		return FileRule{Base: learn.DirLearn, Pattern: "*/" + name, Mode: "0600", Purpose: purpose, Holds: holds,
			NeverHolds: never, RemovedBy: "purge", Owner: "learn"}
	}
	return []FileRule{
		{
			Base:       learn.DirLearn,
			Pattern:    learn.FileKey,
			Mode:       "0600",
			Purpose:    "the key state.sum is made with, written when a lesson is first saved",
			Holds:      "32 random bytes",
			NeverHolds: "anything about a call, a session or a lesson",
			RemovedBy:  "purge",
			Owner:      "learn",
		},
		project(learn.FileLessons, "the project's lessons, read by `rashomon learn`",
			"each lesson's id, tier, text, when it entered and when it was last approved, and when the lessons were last quarantined",
			"a command, a tool's output or a session id"),
		project(learn.FileLabels, "what each fire of a lesson came to, from which its tier is computed",
			"per fire: the lesson's id, the session's id, the time and confirmed, refuted or unresolved",
			"a command, a tool's output or a lesson's text"),
		project(learn.FileActive, "the approved lessons recap gives the agent with a prompt, under 64 KiB",
			"each approved lesson's id, tier, text, when it entered and when it was approved, and whether the lessons are quarantined",
			"a lesson no person approved"),
		project(learn.FileSum, "tamper evidence, not secrecy: an HMAC over the three files above, so a write rashomon did not make is detected and sends every lesson back to proposed, unless the writer can also read the store key (the same limit as spec trust)",
			"one hex digest", "anything about a call, a session or a lesson"),
		{
			Base:       learn.DirLearn,
			Pattern:    "." + learn.FileKey + ".tmp-*",
			Mode:       "0600",
			Purpose:    "the key's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what key is about to hold",
			NeverHolds: "anything about a call, a session or a lesson",
			RemovedBy:  "the write that made it (renamed over key); purge",
			Owner:      "learn",
		},
		project(".*.tmp-*", "a state file's replacement; present only for the instant between the write and the rename, or after a crash in between",
			"what the file it replaces is about to hold", "anything that file never holds"),
	}
}
