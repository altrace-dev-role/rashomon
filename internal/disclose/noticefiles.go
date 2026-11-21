package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// noticeFiles is the run segment's notice markers (store.TakeNotice) under
// runSegment, apart from runSegmentFiles for Law 10 (card VERSION-GATE1): the
// steer router's two, then the Stop's coverage notice and its cleared line,
// then the prompt's index cap notice (READ-SAFE1a c5), then the post's
// markers for a kind of file no reader here claims (kindNoticeFiles,
// LANG-REG1).
func noticeFiles(runSegment string) []FileRule {
	return append([]FileRule{
		{
			Base:       runSegment,
			Pattern:    store.FileNoticeChannel,
			Mode:       "0600",
			Purpose:    "says the session has printed its one line that a note was built and not sent to the agent because the Claude Code version's channel is not verified",
			Holds:      "nothing; present once that line has printed",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileNoticeHoldout,
			Mode:       "0600",
			Purpose:    "says the session has printed its one line that it is in the measurement holdout",
			Holds:      "nothing; present once that line has printed",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileNoticeCoverage,
			Mode:       "0600",
			Purpose:    "says the session has printed its one line that its claim checks are off while rashomon's record of it is not complete",
			Holds:      "nothing; present once that line has printed",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileNoticeCoverageClear,
			Mode:       "0600",
			Purpose:    "says the session has printed its one line that its claim checks are on again",
			Holds:      "nothing; present once that line has printed",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileNoticeIndexCap,
			Mode:       "0600",
			Purpose:    "says the session has printed its one line that the repository index is over its size cap, so its prompts get no brief from it",
			Holds:      "nothing; present once that line has printed",
			NeverHolds: "anything",
			RemovedBy:  "size-cap eviction, with its run; purge",
			Owner:      "store",
		},
	}, kindNoticeFiles(runSegment)...)
}
