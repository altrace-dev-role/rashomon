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
func RunProbe(sig *Signals, phase string, in io.Reader, st *store.Store, now func() time.Time) {
	sessionID := UnattributedSession
	reason := ""

	err := safe.Guard(func() error {
		raw, err := readPayload(in)
		if err != nil {
			return err
		}
		var p sessionPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		if p.SessionID != "" {
			sessionID = p.SessionID
		}

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
		return st.AppendCoverage(BuildCoverage(st, sessionID, phase, reason, now()))
	})

	// Eviction runs at session boundaries, off the per-call path. The session
	// that is starting or ending is never a candidate.
	_ = safe.Guard(func() error {
		_, err := st.Evict(store.CapBytes(), sessionID, now())
		return err
	})
}
