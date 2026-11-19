package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// frontierRunFiles is the inventory's rows for the session graph's frontier
// (store.SaveFrontier) and the temporary file its save renames over it,
// apart from defaultFiles for Law 10 as overlayRunFiles is.
func frontierRunFiles() []FileRule {
	const runSegment = store.DirRuns + "/<session>"
	return []FileRule{
		{
			Base:       runSegment,
			Pattern:    store.FileFrontier,
			Mode:       "0600",
			Purpose:    "the session graph's frontier: the fold of the run's records.ndjson and steer.ndjson through a mark, kept so a later hook folds only the lines after it instead of the whole run",
			Holds:      "the frontier's schema, fold and ledger versions and the place of the last line it folded (a seq, whether it was a steer line, and a steer line's clock and id); the session id; for each prompt, its node id (the session id and the prompt's ordinal), the prompt id the steer line holds, the seq it was stated at and its status; for each requirement a delivered line stated, its predicate, its argument as the line holds it (a graph line's subject: the path its line holds (a code line's path, never a symbol), its keyed digest at record scope v4, or a decision's knowledge id), its key (the predicate and the argument), its status, the seqs it was stated, last stated and met at, and the node id of the prompt that superseded it; at record scope v5 only, the project-relative paths of the files the session's calls named as written, in the clear, with a count of the write declarations the record withheld and a count of those that named a path outside the project; the last ledger line's obligations as it carried them (id, kind, a unit's global id or its digest below record scope v5, and state), that line's seq and its two counts of rows left off; for each steer line delivered, its id, key, kind and seq, where a key can carry a package path of the project (a blast-radius finding keys on the package and a digest); each Stop's verdict by the seq of its checkpoint (a done flag and a count of unmet obligations) and the seqs of the Stops with no verdict logged; the seq of the last compact or resume; the byte offsets the fold read records.ndjson and steer.ndjson through, their sizes then and the size of the store's gaps.ndjson then; and, of the records.ndjson line that ends at that offset, its length in bytes and a SHA-256 digest (64 hex digits) of those bytes, computed from them alone, never the line. The run a clear continued and the decisions are empty in this build",
			NeverHolds: "a prompt's or a message's words, a command's text, file contents or any tool output: every string it holds is one of the ids, digests, closed names and paths above; and, among the written files, a path the record withheld or one outside the project",
			RemovedBy:  "forget --since, --before or --host, for every run it changes, before any of the run's lines go: after a forget, what the frontier drew from the steer log returns at the next fold, and what it drew from the records does not; size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    "." + store.FileFrontier + ".tmp-*",
			Mode:       "0600",
			Purpose:    "frontier.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what frontier.json holds",
			NeverHolds: "what frontier.json never holds",
			RemovedBy:  "the save that wrote it (renamed over frontier.json); forget --since, --before or --host, with the frontier; size-cap eviction, with its run; purge",
			Owner:      "store",
		},
	}
}
