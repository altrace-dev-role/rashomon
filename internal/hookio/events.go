package hookio

import "github.com/altrace-dev-role/rashomon/internal/events"

// EventKind maps a Claude Code hook event and, for SessionStart, its source
// to the canonical kind and form (events.Event): the one place the agent's
// event names meet the core's vocabulary (AD9). A SessionStart whose source
// is compact, resume or clear is a boundary of that form; any other source
// (startup, or none) is a session start carrying the source word. The
// subagent events map to boundaries ahead of their subscription (W-M4.3).
// ok is false for an event this adapter does not map.
func EventKind(hookEvent, source string) (kind events.Kind, form string, ok bool) {
	switch hookEvent {
	case "SessionStart":
		switch source {
		case events.Compact, events.Resume, events.Clear:
			return events.Boundary, source, true
		}
		return events.SessionStart, source, true
	case "UserPromptSubmit":
		return events.Prompt, "", true
	case "PreToolUse":
		return events.ActDeclared, "", true
	case "PostToolUse":
		return events.ActLanded, "", true
	case "PostToolUseFailure":
		return events.ActFailed, "", true
	case "Stop":
		return events.Stop, "", true
	case "StopFailure":
		return events.StopFailed, "", true
	case "SessionEnd":
		return events.SessionEnd, "", true
	case "SubagentStart":
		return events.Boundary, events.SubagentStart, true
	case "SubagentStop":
		return events.Boundary, events.SubagentStop, true
	}
	return "", "", false
}
