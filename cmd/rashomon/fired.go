package main

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// markFired records that an entry ran for event in the session, the marker
// status counts the sessions each event fired in from. It is best effort, as
// everything an entry does past its records: an entry never fails over its
// own bookkeeping, and a payload that names no session marks nothing.
func markFired(st *store.Store, sessionID, event string) {
	if sessionID != "" {
		_ = st.MarkFired(sessionID, event, time.Now())
	}
}

// sessionOf is the payload's session_id, "" with no payload or none in it.
func sessionOf(payload *hookio.Payload) string {
	if payload == nil {
		return ""
	}
	id, _ := payload.Str(hookio.SessionID)
	return id
}

// postEvent is the event the post entry ran for: the one command serves both
// of a tool call's outcomes, and the payload names which.
func postEvent(payload *hookio.Payload) string {
	if payload != nil {
		if name, _ := payload.Str(hookio.HookEventName); name == "PostToolUseFailure" {
			return name
		}
	}
	return "PostToolUse"
}

// probeEvent is the event the probe entry ran for, by its phase.
func probeEvent(phase string) string {
	if phase == store.PhaseEnd {
		return "SessionEnd"
	}
	return "SessionStart"
}

// recapEvent is the event a recap entry ran for: the catch-up's
// UserPromptSubmit, and for a turn's end the StopFailure the payload names,
// else Stop.
func recapEvent(args []string, name string) string {
	switch {
	case recapPending(args):
		return "UserPromptSubmit"
	case name == "StopFailure":
		return name
	}
	return "Stop"
}
