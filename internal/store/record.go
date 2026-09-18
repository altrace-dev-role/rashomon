package store

import "github.com/altrace-dev-role/rashomon/internal/shape"

// SchemaVersion is carried by every record. A reader that does not recognise it
// should skip the record rather than guess at it.
//
// v2 is additive over v1: declaration gained hosts and ssh_hosts, execution
// gained outcome, exit_code, is_interrupt and duration_ms. No v1 field changed
// meaning and none was removed, which is what makes reading both safe.
const SchemaVersion = 2

// Accepts reports whether a reader understands a record's schema version.
//
// It is a function over a set rather than an equality against SchemaVersion,
// because the two are different questions and conflating them is a silent
// data-loss bug: the reader used to compare `!= SchemaVersion`, so the moment
// the writer moved to v2 every v1 record already on disk would have been
// skipped -- a store that had been recording for weeks would have rendered an
// empty report, and nothing would have said why.
func Accepts(version int) bool {
	return version == 1 || version == 2
}

// Record type discriminators.
const (
	TypeDeclaration = "declaration"
	TypeExecution   = "execution"
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

	// Hosts and SSHHosts are the hostnames this call NAMED (v2).
	//
	// They are a declaration, not an observation: a host here is one the agent
	// wrote down, whether or not the call ran and whether or not the wire ever
	// saw it. That is the entire point -- the report's central line compares
	// this list against the proxy's, and "reached but never named" is only
	// meaningful because this side is the naming side.
	//
	// They are separate lists because the two have opposite fates in the
	// report. A wire-observable host with no record is a finding; an ssh host
	// with no record is expected, since the proxy cannot see ssh at all, and it
	// renders under coverage as "not observable" rather than as something the
	// session hid. Merging them and filtering later is how the second kind ends
	// up printed as the first.
	//
	// Both are sorted and de-duplicated by the extractor so two runs of one
	// command produce byte-identical records. Null when the call named none;
	// never an empty array, because "we looked and found none" and "we did not
	// look" must stay distinguishable for a v1 record read back.
	Hosts    []string `json:"hosts"`
	SSHHosts []string `json:"ssh_hosts"`
}

// Execution records that a declared call ran, one per PostToolUse invocation.
//
// A declaration with an execution beside it ran. A declaration without one was
// denied, failed, or had its execution go unrecorded, and nothing here claims
// to know which of the three it was.
//
// It carries the join key, the clock and the tool name, and no field that could
// hold any part of a tool response. The PostToolUse payload carries the
// response; internal/hook does not declare a field for it, so it is never a
// value in this process, and there is nowhere here for it to be put if it were.
//
// Seq is null when the record was written to the spill file because the
// ordered stream's lock could not be taken. The id lands either way; what a
// lock timeout costs is the position in the total order, not the record.
type Execution struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	Seq           *int64 `json:"seq"`
	RecordedAtMS  int64  `json:"recorded_at_unix_ms"`
	ToolUseID     string `json:"tool_use_id"`
	SessionID     string `json:"session_id"`
	ToolName      string `json:"tool_name"`

	// Outcome is how the call ended (v2): one of ExecOK, ExecFailed,
	// ExecInterrupted.
	//
	// It is a plain string, not a pointer, and the empty value is load-bearing:
	// a v1 record read back has no outcome, and the report must render that
	// tool_use_id as "outcome unobserved" rather than as a success. Defaulting
	// an unknown outcome to ok is the exact failure the product exists to
	// prevent -- it would turn a call whose ending nobody recorded into a call
	// that is claimed to have worked.
	Outcome string `json:"outcome"`

	// ExitCode is the process exit status, parsed from the failure payload's
	// `error: "Exit code N"` (v2).
	//
	// Null on success and on any other error shape, and NEVER 0: zero is an
	// exit status meaning success, so writing it for "we could not parse one"
	// would assert the opposite of what happened. A failure whose message does
	// not carry a code is a failure with an unknown code, which is null.
	ExitCode *int `json:"exit_code"`

	// IsInterrupt distinguishes a user interrupt from a command that failed on
	// its own (v2). Null when the payload did not say, which for a successful
	// call is always.
	IsInterrupt *bool `json:"is_interrupt"`

	// DurationMS is how long the call took, as the client measured it (v2).
	// Null when absent; never 0, for the same reason as ExitCode.
	DurationMS *int64 `json:"duration_ms"`
}

// Execution outcomes (v2).
//
// Deliberately NOT the same vocabulary as the Terminal outcomes below. A
// Terminal describes how the RECORDER's own invocation ended (ok, error,
// signal); these describe how the TOOL CALL ended. They are different facts
// about different processes, and one shared vocabulary would let a report
// sentence about the agent's behaviour be built out of a fact about the
// recorder's.
const (
	ExecOK          = "ok"
	ExecFailed      = "failed"
	ExecInterrupted = "interrupted"
)

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

// Reasons lists every coverage reason code a record can carry. It exists so
// that the store schema can be checked against the vocabulary the code writes
// rather than against a copy of it that drifts.
func Reasons() []string {
	return []string{
		ReasonInternalError,
		ReasonLockTimeout,
		ReasonTerminatedBySignal,
		ReasonUnterminatedEntry,
		ReasonHookEntryAbsent,
		ReasonHookEntryUnresolved,
		ReasonProbeAbsent,
		ReasonProbeUnresolved,
	}
}

// Coverage phases: which hook wrote the record.
const (
	PhaseStart = "start"
	PhaseCall  = "call"
	PhasePost  = "post"
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
