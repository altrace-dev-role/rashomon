package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// kindNoticeFiles is the post's markers for a kind of file no reader here
// claims (store.TakeKindNotice, card LANG-REG1) under runSegment: one per
// kind up to 16, then the one for the kinds no line names; then its markers
// for a language the index holds no records of, one per language; apart
// from noticeFiles for Law 10 as blastReadFiles is.
func kindNoticeFiles(runSegment string) []FileRule {
	return []FileRule{
		{
			Base:    runSegment,
			Pattern: store.FileNoticeKindPrefix + "*",
			Mode:    "0600",
			Purpose: "one per kind of file, up to 16, that the session edited and no reader here claims; says the session " +
				"has printed its one line that edits to that kind carry no unit, caller or conflict facts",
			Holds:      "nothing; its name is the start of the sha256 of the kind's extension or file name, which keeps the name safe, not secret",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:    runSegment,
			Pattern: store.FileNoticeOtherKinds,
			Mode:    "0600",
			Purpose: "says the session has printed its one line that some other kinds of file have no reader here: " +
				"kinds past the 16 named, and kinds whose name the line does not print",
			Holds:      "nothing; present once that line has printed",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:    runSegment,
			Pattern: store.FileNoticeUnindexedPrefix + "*",
			Mode:    "0600",
			Purpose: "one per language whose files the session edited where the index here holds no records of it; says the " +
				"session has printed its one line that edits to those files carry no unit, caller or conflict facts until the index runs",
			Holds:      "nothing; its name is the start of the sha256 of the language's name",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
	}
}
