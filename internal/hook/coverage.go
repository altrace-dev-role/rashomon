package hook

import (
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/install"
	"github.com/altrace-dev-role/altrace-attest/internal/settings"
	"github.com/altrace-dev-role/altrace-attest/internal/store"
)

// Resolution is the configuration state as this process sees it, at the moment
// it looks. It is recorded, never recomputed later: a run is judged by what was
// true while it ran, so that detach cannot rewrite the past.
type Resolution struct {
	HookEntry string
	Probe     string
}

// Resolve reads the user's settings file and the probe marker.
//
// hook_entry is "present" only when our PreToolUse entry is installed as watch
// installs it, with our matcher and our timeout. An entry that exists but has
// been narrowed to "Bash" is "absent" for coverage purposes, because that is
// what it is: a config under which most tool calls produce no declaration.
//
// Every failure resolves to "unknown". That is a real answer and a worse one
// than "present", which is the point: a handler that cannot read its own
// configuration must not claim to know it.
func Resolve(st *store.Store, sessionID string) Resolution {
	res := Resolution{HookEntry: store.EntryUnknown, Probe: store.ProbeUnknown}

	if path, err := settings.UserPath(); err == nil {
		if doc, err := settings.Load(path); err == nil {
			if present, err := install.Present(doc, st.InstallID(), install.EventPreToolUse); err == nil {
				res.HookEntry = store.EntryAbsent
				if present {
					res.HookEntry = store.EntryPresent
				}
			}
		}
	}

	if fresh, err := st.ProbeFresh(sessionID); err == nil {
		res.Probe = store.ProbeAbsent
		if fresh {
			res.Probe = store.ProbeFresh
		}
	}
	return res
}

// BuildCoverage assembles the coverage record for one hook invocation. A
// failure reason from the invocation itself takes precedence over anything
// resolved from configuration; after that, the entry's state outranks the
// probe's, because an absent entry explains an absent probe and not the other
// way round.
func BuildCoverage(st *store.Store, sessionID, phase, reason string, now time.Time) store.Coverage {
	res := Resolve(st, sessionID)
	if phase == store.PhaseStart {
		// This invocation is the probe; it has just marked itself.
		res.Probe = store.ProbeFresh
	}

	if reason == "" {
		switch {
		case res.HookEntry == store.EntryAbsent:
			reason = store.ReasonHookEntryAbsent
		case res.HookEntry == store.EntryUnknown:
			reason = store.ReasonHookEntryUnresolved
		case res.Probe == store.ProbeAbsent:
			reason = store.ReasonProbeAbsent
		case res.Probe == store.ProbeUnknown:
			reason = store.ReasonProbeUnresolved
		}
	}

	state := store.StateVerified
	if reason != "" {
		state = store.StateUnverified
	}
	return store.Coverage{
		Type:          store.TypeCoverage,
		SchemaVersion: store.SchemaVersion,
		RecordedAtMS:  now.UnixMilli(),
		SessionID:     sessionID,
		InstallID:     st.InstallID(),
		Phase:         phase,
		State:         state,
		Reason:        nilIfEmpty(reason),
		HookEntry:     res.HookEntry,
		Probe:         res.Probe,
	}
}
