package hook

import (
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// RunProbe handles a SessionStart or SessionEnd invocation.
//
// At start it marks the session as probed and records the resolved config
// state. At end it records it again, this time having read the run's records
// to see whether every declaration was closed -- so that a handler killed
// mid-flight is written into the run's own coverage while the run is still
// live, rather than inferred later by a report that has to guess.
//
// It never fails the hook. A probe that cannot do its job records that it
// could not, and returns. The one failure no record can hold is eviction's,
// which is about the store rather than this session: it is returned, for the
// caller to say so on stderr.
func RunProbe(sig *Signals, phase string, in io.Reader, st *store.Store, now func() time.Time) error {
	sessionID := UnattributedSession
	reason := ""
	cwd := ""

	err := safe.Guard(func() error {
		payload, err := hookio.Intake(in)
		if err != nil {
			if s, c, ok := refusedSession(err); ok {
				sessionID, cwd = s, c
			}
			return err
		}
		var p sessionPayload
		if err := p.from(payload); err != nil {
			return err
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
		reason = closeReason(err)
	}

	// A start or end coverage record that cannot be written is read as its
	// absence: probe_absent at start, run_not_closed at end (report.build).
	_ = safe.Guard(func() error {
		return st.AppendCoverage(BuildCoverage(st, sessionID, phase, reason, cwd, now()))
	})

	// Eviction runs at session boundaries, off the per-call path. The session
	// that is starting or ending is never a candidate. Its error is not this
	// session's, so it goes to no coverage record; Evict writes each gap
	// before removing its run, so whatever it did remove is accounted for.
	return safe.Guard(func() error {
		_, err := st.Evict(store.CapBytes(), sessionID, now())
		return err
	})
}
