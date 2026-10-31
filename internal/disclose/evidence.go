package disclose

import "github.com/altrace-dev-role/rashomon/internal/store"

// evidenceFiles is the inventory's row for the channel-evidence marker (card
// CHANNEL-EVIDENCE1), apart from defaultFiles for Law 10, as learnFiles and
// specFiles are.
func evidenceFiles() []FileRule {
	return []FileRule{
		{
			Base:       "",
			Pattern:    store.ChannelEvidencePrefix + "*",
			Mode:       "0600",
			Purpose:    "says a Stop read Claude Code's transcript and did not find a note rashomon delivered for that hook event, so the event's notes are not sent until a probe passes or Claude Code is newer; one file per event",
			Holds:      "the Claude Code version that shadowed the event, as dotted digits",
			NeverHolds: "anything from the transcript, a note's text, or a session id",
			RemovedBy:  "the first routed call on a newer Claude Code version; test/live/p-tru-1.sh's printed command after a pass; purge",
			Owner:      "store",
		},
	}
}
