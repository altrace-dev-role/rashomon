package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// SessionContentScope is the scope_version whose screen names the session's
// content kept under the store's graph directory (ruling 15, W-M2.11 step
// 1d): a consent on file at this scope or later, at record scope v5, is what
// consent.SessionContent reads as yes. It is its own constant, not
// ScopeVersion, so a later scope does not move it.
const SessionContentScope = 6

// sessionContentFiles is the inventory's rows for the session-content
// marker, the third view of consent.json (internal/consent), and for the two
// kinds of file at the top of the store's graph directory: the session log
// the hook appends while the marker says yes, and the evidence file the
// prompt's graph brief writes at every prompt in an indexed repository
// while the record scope is v5, session_content or not. The directory is
// graph.StoreDir and the names are those of
// graph's sessionLogFile and EvidenceFileName, written here as literals since
// disclose does not import graph (unitBriefFiles' reason); purge reads them
// back with graph.IsSessionLogName and graph.IsEvidenceFileName.
func sessionContentFiles() []FileRule {
	const prompt = "any word of the prompt"
	return []FileRule{
		{
			// Every consent write removes it, appends to consent.json and
			// writes it again from consent.json's answer, and every session
			// start puts it right (internal/consent, internal/hook).
			Base:       "",
			Pattern:    store.FileSessionContent,
			Mode:       "0600",
			Purpose:    "says the consent on file covers the session's content (scope 6 or later at record scope v5), kept beside consent.json so a hook call reads one small file instead of parsing it; no marker means none of the session's content is kept",
			Holds:      "1",
			NeverHolds: "anything else",
			RemovedBy:  "every consent write, which writes it again after while the consent on file covers it; purge",
			Owner:      "consent",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileSessionContent + ".tmp-*",
			Mode:       "0600",
			Purpose:    "session_content's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "1",
			NeverHolds: "anything else",
			RemovedBy:  "the write that made it (renamed over session_content); purge",
			Owner:      "consent",
		},
		{
			// The log's Prompt, Call, Claim, Note and Verdict records have no
			// writer in this tree; the card that adds one rewrites Holds and
			// NeverHolds here.
			Base:    "graph",
			Pattern: "*.jsonl",
			Mode:    "0600",
			Purpose: "the session log of one repository indexed with `rashomon graph index`, named by 16 hex of the SHA-256 of its id, " +
				"appended at each Write, Edit or MultiEdit of a Go file there while the session's content is kept; never rewritten, " +
				"and the size cap does not cover it",
			Holds: "a header with its format's version, the repository's id, the graph's commit and its writer's name and version; " +
				"then, for each such call, once from its input before it runs and once from the file after it: the session, the " +
				"call's id, the file's path in the repository, which of the two it is, the call's sequence number, the file's " +
				"SHA-256 and language (go), and what the file then declares (each function, type, method, constant and variable " +
				"by name and kind, the first sentence of its doc comment, the names a function or method calls, and the packages " +
				"the file imports), with the count of its records, the kinds of link its reader records, whether its declarations " +
				"were cut at the cap and, when the file after the call has the content its input gave, the sequence number of the " +
				"one from its input; each record by its ids, its kind or predicate, its tier, confidence and status, and the " +
				"commit and sequence number it belongs to",
			NeverHolds: "a function's signature or any other text of the file, and " + prompt,
			RemovedBy:  "purge",
			Owner:      "graph",
		},
		{
			Base:    "graph",
			Pattern: "*-*.txt",
			Mode:    "0600",
			Purpose: "the evidence for one prompt's graph brief, written at each prompt in a repository with .rashomon/graph.jsonl " +
				"while the store's record scope is v5, and named by the session and the prompt (an older build wrote it at " +
				"v4 too, and purge removes those as well); the size cap does not cover it",
			Holds: "the scorer and the count of the prompt's words; the graph's commit; the ids the brief scored, reached, chose " +
				"and rendered, with their scores and, for each one scored, the count of the prompt's words it matched; and the " +
				"brief's text",
			NeverHolds: prompt,
			RemovedBy:  "purge",
			Owner:      "cmd",
		},
	}
}
