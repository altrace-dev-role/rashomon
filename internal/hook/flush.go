package hook

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Flush is the group commit's one full sync for this invocation (REC-34),
// issued after Close's last append and before the process exits. It returns
// an error only when the flush failed and the marker saying so could not be
// written either; see flushOrMark.
func (h *Handler) Flush() error {
	return flushOrMark(h.st, h.sessionID, h.toolUseID, h.now())
}

// Flush is Handler.Flush for a PostToolUse invocation.
func (p *Post) Flush() error {
	return flushOrMark(p.st, p.sessionID, p.toolUseID, p.now())
}

// flushOrMark issues the store's one full sync and, when it fails, marks the
// call instead of retrying: a spill terminal carrying store.ReasonFlushFailed,
// the one record this process can append without waiting on a lock, so a
// reader takes the call's records as possibly not durable rather than as
// fine. The marker is synced in full, because a failed Flush ends the store's
// deferral: behind a barrier alone it could be lost with the records it
// speaks for, and say nothing.
//
// It returns an error when the marker could not be written or its full sync
// failed: a marker that is not durable is not a marker. There is then no
// record left to hold the failure, and the caller says so on stderr, as
// openForHook does when there is no store.
func flushOrMark(st *store.Store, sessionID, toolUseID string, now time.Time) error {
	if st.Flush() == nil {
		return nil
	}
	reason := store.ReasonFlushFailed
	return st.SpillTerminal(store.Terminal{
		Type:          store.TypeTerminal,
		SchemaVersion: store.SchemaVersion,
		RecordedAtMS:  now.UnixMilli(),
		ToolUseID:     toolUseID,
		SessionID:     sessionID,
		Outcome:       store.OutcomeError,
		Reason:        &reason,
	})
}
