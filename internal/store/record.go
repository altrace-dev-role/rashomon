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
	TypeGap         = "gap"
)

// Files within a run directory, and at the store root.
const (
	FileRecords  = "records.ndjson"
	FileSpill    = "spill.ndjson"
	FileCoverage = "coverage.ndjson"
	FileProbe    = "probe"
	FileSeq      = "seq"
	FileGaps     = "gaps.ndjson"
)

// Declaration is what the agent asked to run.
//
// It is not an execution. PreToolUse fires before the permission flow resolves,
// so a declaration exists for a call the user went on to deny; PermissionMode
// is carried so that a later comparison can exclude those rather than having to
// guess which ones ran.
//
// Seq is allocated under the run's append lock and is strictly increasing
// within records.ndjson. It is what gives the file a total order that a
// millisecond clock cannot, and it is the invariant the lock exists to protect.
//
// Every nullable field here is a pointer because the absent case is real and
// has to stay distinguishable: PromptID is absent until the first input of a
// session, and AgentID and AgentType are present only inside a subagent call.
// An empty string would record "we saw a value and it was empty", which is a
// different and false claim.
type Declaration struct {
	Type           string      `json:"type"`
	SchemaVersion  int         `json:"schema_version"`
	Seq            int64       `json:"seq"`
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
// of the uncontrolled path. That absence is the only honest record of it.
//
// Seq is null when the record was written to the spill file because the
// ordered stream's lock could not be taken. Such a record still closes its
// declaration -- or, when the declaration itself was what could not be
// written, is the only trace of it, which is why it carries the tool_use_id.
type Terminal struct {
	Type          string  `json:"type"`
	SchemaVersion int     `json:"schema_version"`
	Seq           *int64  `json:"seq"`
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
	ReasonInternalError       = "internal_error"
	ReasonLockTimeout         = "lock_timeout"
	ReasonTerminatedBySignal  = "terminated_by_signal"
	ReasonUnterminatedEntry   = "unterminated_entry"
	ReasonHookEntryAbsent     = "hook_entry_absent"
	ReasonHookEntryUnresolved = "hook_entry_unresolved"
	ReasonProbeAbsent         = "probe_absent"
	ReasonProbeUnresolved     = "probe_unresolved"
)

// Coverage phases: which hook wrote the record.
const (
	PhaseStart = "start"
	PhaseCall  = "call"
	PhaseEnd   = "end"
)

// Resolved states of the installed hook entry and of the liveness probe, as
// they were at the moment the record was written.
const (
	EntryPresent = "present"
	EntryAbsent  = "absent"
	EntryUnknown = "unknown"

	ProbeFresh   = "fresh"
	ProbeAbsent  = "absent"
	ProbeUnknown = "unknown"
)

// Coverage records the resolved configuration state as it was when the handler
// ran. report reads these; it never re-reads today's configuration to judge a
// past run, because that would let detach retroactively invalidate every run
// the user had ever captured.
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
	Phase         string  `json:"phase"`
	State         string  `json:"state"`
	Reason        *string `json:"reason"`
	HookEntry     string  `json:"hook_entry"`
	Probe         string  `json:"probe"`
}

// Gap reasons.
const (
	GapForget  = "forget"
	GapSizeCap = "size_cap"
)

// Gap records that records left the store, and why. It is the only way records
// leave: a deletion with no gap record is a silent deletion, and a store that
// can be silently edited is worth nothing as evidence.
type Gap struct {
	Type           string `json:"type"`
	SchemaVersion  int    `json:"schema_version"`
	RecordedAtMS   int64  `json:"recorded_at_unix_ms"`
	SessionID      string `json:"session_id"`
	Reason         string `json:"reason"`
	FromUnixMS     int64  `json:"from_unix_ms"`
	ToUnixMS       int64  `json:"to_unix_ms"`
	RemovedRecords int    `json:"removed_records"`
}
