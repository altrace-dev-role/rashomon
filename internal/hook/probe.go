package hook

import (
	"io"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// RunProbe handles a SessionStart or SessionEnd invocation.
//
// At start it marks the session as probed, sets the record-scope and
// steer-disclosure markers from consent.json (syncRecordScope and
// syncSteerDisclosure: the only reads of consent.json by any hook, at
// SessionStart only) and records the resolved config state. At end it records it again, this time having read the run's records
// to see whether every declaration was closed -- so that a handler killed
// mid-flight is written into the run's own coverage while the run is still
// live, rather than inferred later by a report that has to guess.
//
// Its lock waits share one deadline, LockWaitTotal, as a hook's do.
//
// It never fails the hook. A probe that cannot do its job records that it
// could not, and returns. The one failure no record can hold is eviction's,
// which is about the store rather than this session: it is returned, for the
// caller to say so on stderr.
func RunProbe(sig *Signals, phase string, in io.Reader, st *store.Store, now func() time.Time) error {
	return runProbe(sig, phase, in, st, now, LockWaitTotal)
}

// RunProbeFromPayload is RunProbe for a caller that already has a parsed
// *hookio.Payload in hand -- cmd/rashomon's host-guard check, which hands
// this the exact value internal/hostguard.EvaluateRaw built, so the bytes
// it already decided on are never read or copied a second time. See
// Handler.CapturePayload's doc, which this mirrors.
func RunProbeFromPayload(sig *Signals, phase string, payload *hookio.Payload, st *store.Store, now func() time.Time) error {
	return runProbeFromPayload(sig, phase, payload, nil, st, now, LockWaitTotal)
}

// runProbe is RunProbe with its lock waits bounded by waits, which a test
// shortens.
func runProbe(sig *Signals, phase string, in io.Reader, st *store.Store, now func() time.Time, waits time.Duration) error {
	return runProbeFromPayload(sig, phase, nil, in, st, now, waits)
}

// runProbeFromPayload is runProbe and RunProbeFromPayload's shared body.
// Exactly one of payload and in is non-nil: payload when the caller has
// already read and parsed it, in when it still owns an unread io.Reader
// and runProbeFromPayload must call hookio.Intake itself.
func runProbeFromPayload(sig *Signals, phase string, payload *hookio.Payload, in io.Reader, st *store.Store, now func() time.Time, waits time.Duration) error {
	st.LimitLockWaits(waits)
	sessionID := store.UnattributedSession
	reason := ""
	cwd := ""
	transcriptPath := ""

	err := safe.Guard(func() error {
		if payload == nil {
			var err error
			payload, err = hookio.Intake(in)
			if err != nil {
				if s, c, ok := refusedSession(err); ok {
					sessionID, cwd = s, c
				}
				return err
			}
		}
		var p sessionPayload
		if err := p.from(payload); err != nil {
			return err
		}
		sessionID = attribute(p.SessionID)
		cwd = p.CWD

		switch phase {
		case store.PhaseStart:
			if err := st.MarkProbe(sessionID, now()); err != nil {
				return err
			}
			// The record scope the session's calls apply, and the steer
			// disclosure they check consent against, set from consent.json
			// once here so that no call has to read it (syncRecordScope,
			// syncSteerDisclosure). Each runs whatever the other did; only
			// the record scope's failure names a reason.
			if !syncRecordScope(st.Root()) {
				reason = store.ReasonRecordScopeUnresolved
			}
			syncSteerDisclosure(st.Root())
			return nil
		case store.PhaseEnd:
			// Read here alone, and a path that is not a string is no copy:
			// sessionPayload's fields fail the whole payload on a wrong type,
			// and a probe that never read this field must not start failing.
			transcriptPath, _ = payload.Str(hookio.TranscriptPath)
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

	// The transcript copy (card 4.1, D111 as amended) runs after the end
	// record, which so neither waits on it nor is lost to it, in a barrier of
	// its own. A refused payload (hookio.TooLargeError) never reaches the
	// PhaseEnd case, so it has no path and no copy. The copy's error has no
	// reader yet: this function's is eviction's alone, which cmdProbe prints
	// as evictionLine, and card 4.1b gives the copy a line of its own.
	if phase == store.PhaseEnd && transcriptPath != "" {
		_ = safe.Guard(func() error {
			key := baseline.Key(cwd)
			_, err := st.CopyBody(sessionID, transcriptPath, key, strings.TrimSuffix(baseline.FileName(key), baseline.FileExt), now())
			return err
		})
	}

	// Eviction runs at session boundaries, off the per-call path. The session
	// that is starting or ending is never a candidate. Its error is not this
	// session's, so it goes to no coverage record; Evict writes each gap
	// before removing its run, so whatever it did remove is accounted for.
	//
	// The cap is resolved here, inside the hook's own environment, and
	// recorded before Evict runs with it: status reads the record rather
	// than EnvCapBytes in whatever shell happens to run it, which can
	// disagree with what Claude Code hands the hooks (hld-7).
	capSetting := store.ResolveCap()
	_ = st.RecordResolvedCap(capSetting)
	return safe.Guard(func() error {
		_, err := st.Evict(capSetting.Bytes, sessionID, now())
		return err
	})
}
