package hook

import (
	"encoding/json"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// sessionPayload is the part of a SessionStart or SessionEnd payload this
// program reads. The rest -- source, reason, cwd -- is not needed and not kept.
type sessionPayload struct {
	SessionID string `json:"session_id"`
	// ConversationID is read for one purpose: to recognise a session start or
	// end that is not Claude Code's. See RunProbe.
	ConversationID string `json:"conversation_id"`
	// CWD is read now: the novelty baseline is per project and the project is
	// derived from this path. The rest of a session payload -- source, reason
	// -- is still not needed and not kept.
	CWD string `json:"cwd"`
}

// RunProbe handles a SessionStart or SessionEnd invocation.
//
// At start it marks the session as probed and records the resolved config
// state. At end it records it again, this time having read the run's records
// to see whether every declaration was closed -- so that a handler killed
// mid-flight is written into the run's own coverage while the run is still
// live, rather than inferred later by a report that has to guess.
//
// It never fails. A probe that cannot do its job records that it could not,
// and returns.
//
// A start or end that names a conversation records nothing but eviction; see
// the check below for why.
func RunProbe(sig *Signals, phase string, in io.Reader, st *store.Store, now func() time.Time) {
	sessionID := store.UnattributedSession
	reason := ""
	cwd := ""
	foreign := false

	err := safe.Guard(func() error {
		raw, err := readPayload(in)
		if err != nil {
			return err
		}
		var p sessionPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		// A session start or end that names a CONVERSATION is not Claude
		// Code's, and it records nothing: no probe marker, no coverage record.
		//
		// Per Cursor's documentation -- nobody has run Cursor against this
		// recorder -- its sessionStart and sessionEnd carry a session_id,
		// documented as the same value as conversation_id, while its tool,
		// stop and prompt events carry none. Recorded as a session, that
		// start and end opened and closed a run that no call ever lands in,
		// because the conversation's calls go to the unattributed run: a
		// harness that names its conversation but not its calls got a
		// verified-looking empty session -- coverage verified, zero
		// declarations -- the clean zero over something unwatched that this
		// program exists never to print. Its calls are still counted, as
		// calls that arrived without a session id.
		//
		// The rule rests on an assumption, stated so it can be checked: Claude
		// Code sends no conversation_id today. Its hooks documentation lists
		// session_id among the fields every event receives and conversation_id
		// nowhere, and H-108 pins its documented session start and end as
		// still recording both, so a rule that grows past this field goes red
		// rather than silent. If Claude Code ever sends it, this check is the
		// thing to replace -- with a harness marker in the installed command
		// line, not a guess from the payload.
		if p.ConversationID != "" {
			foreign = true
			return nil
		}
		if p.SessionID != "" {
			sessionID = p.SessionID
		}
		cwd = p.CWD

		switch phase {
		case store.PhaseStart:
			return st.MarkProbe(sessionID, now())
		case store.PhaseEnd:
			run, err := st.ReadRun(sessionID)
			if err != nil {
				return err
			}
			if len(run.Unterminated()) > 0 {
				reason = store.ReasonUnterminatedEntry
			}
		}
		return nil
	})

	switch {
	case sig.Delivered():
		reason = store.ReasonTerminatedBySignal
	case err != nil:
		reason = store.ReasonInternalError
	}

	_ = safe.Guard(func() error {
		if foreign {
			return nil
		}
		return st.AppendCoverage(BuildCoverage(st, sessionID, phase, reason, cwd, now()))
	})

	// Eviction runs at session boundaries, off the per-call path. The session
	// that is starting or ending is never a candidate. A foreign start or end
	// still runs it -- it is the store's own housekeeping, owed at a session
	// boundary whoever's session it is -- and the run it spares is then the
	// unattributed one, which is where that conversation's calls are.
	_ = safe.Guard(func() error {
		_, err := st.Evict(store.CapBytes(), sessionID, now())
		return err
	})
}
