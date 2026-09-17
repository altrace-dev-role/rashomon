package store

import "github.com/altrace-dev-role/altrace-attest/internal/shape"

// SchemaVersion is carried by every record. A reader that does not recognise it
// should skip the record rather than guess at it.
const SchemaVersion = 1

// Record type discriminators.
const (
	TypeDeclaration = "declaration"
	TypeTerminal    = "terminal"
	TypeCoverage    = "coverage"
)

// File names within a run directory.
const (
	FileRecords  = "records.ndjson"
	FileCoverage = "coverage.ndjson"
)

// Declaration is what the agent asked to run.
//
// It is not an execution. PreToolUse fires before the permission flow resolves,
// so a declaration exists for a call the user went on to deny; PermissionMode
// is carried so that a later comparison can exclude those rather than having to
// guess which ones ran.
//
// Every nullable field here is a pointer because the absent case is real and
// has to stay distinguishable: PromptID is absent until the first input of a
// session, and AgentID and AgentType are present only inside a subagent call.
// An empty string would record "we saw a value and it was empty", which is a
// different and false claim.
type Declaration struct {
	Type           string      `json:"type"`
	SchemaVersion  int         `json:"schema_version"`
	RecordedAtMS   int64       `json:"recorded_at_unix_ms"`
	ToolUseID      string      `json:"tool_use_id"`
	SessionID      string      `json:"session_id"`
	PromptID       *string     `json:"prompt_id"`
	AgentID        *string     `json:"agent_id"`
	AgentType      *string     `json:"agent_type"`
	TranscriptPath string      `json:"transcript_path"`
	PermissionMode string      `json:"permission_mode"`
	ToolName       string      `json:"tool_name"`
	Shape          shape.Shape `json:"shape"`
}

// Terminal outcomes.
const (
	OutcomeOK     = "ok"
	OutcomeError  = "error"
	OutcomeSignal = "signal"
)

// Terminal closes a declaration on every exit path the process controls: a
// clean return, an error return, and SIGTERM, which is what a hook timeout
// cancellation delivers and which is catchable.
//
// SIGKILL is not catchable, so a declaration with no Terminal is the signature
// of the uncontrolled path. That absence is the only honest record of it, and
// report reads it as such rather than assuming the run completed.
type Terminal struct {
	Type          string  `json:"type"`
	SchemaVersion int     `json:"schema_version"`
	RecordedAtMS  int64   `json:"recorded_at_unix_ms"`
	ToolUseID     string  `json:"tool_use_id"`
	SessionID     string  `json:"session_id"`
	Outcome       string  `json:"outcome"`
	Reason        *string `json:"reason"`
}

// Coverage states.
const (
	StateVerified   = "verified"
	StateUnverified = "unverified"
)

// Coverage reasons. Fixed codes, never free text, and never derived from input.
const (
	ReasonInternalError      = "internal_error"
	ReasonTerminatedBySignal = "terminated_by_signal"
	ReasonUnterminatedEntry  = "unterminated_entry"
)

// Hook entry states, as resolved at the time the record was written.
const (
	EntryPresent = "present"
	EntryAbsent  = "absent"
	EntryUnknown = "unknown"
)

// Coverage records the resolved configuration state as it was at the moment the
// handler ran. report reads these; it never re-reads today's configuration to
// judge a past run, because that would let detach retroactively invalidate
// every run the user had ever captured.
//
// It carries no count, and the type has no field that could hold one. A run
// whose instrumentation failed must not be able to report a number, and the
// cheapest way to guarantee that is to make the number unrepresentable rather
// than to remember to omit it.
type Coverage struct {
	Type          string  `json:"type"`
	SchemaVersion int     `json:"schema_version"`
	RecordedAtMS  int64   `json:"recorded_at_unix_ms"`
	SessionID     string  `json:"session_id"`
	InstallID     string  `json:"install_id"`
	State         string  `json:"state"`
	Reason        *string `json:"reason"`
	HookEntry     string  `json:"hook_entry"`
}
