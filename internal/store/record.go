package store

import "github.com/altrace-dev-role/rashomon/internal/shape"

// SchemaVersion is carried by every record. A reader that does not recognise it
// should skip the record rather than guess at it.
//
// v2 is additive over v1: declaration gained hosts and ssh_hosts; execution
// gained outcome, exit_code, is_interrupt, duration_ms and executed_digest;
// coverage gained cwd. No v1 field changed meaning and none was removed, which
// is what makes reading both safe.
//
// v3 is additive over v2 as well: declaration gained file_label, host_source,
// rule_match, cwd_digest and shape.may_write; execution gained rule_match and
// backgrounded. And the test verb class, with the test-file label, is a v3
// vocabulary: a v2 record says package or execute for `go test`, so a reader
// that meets one knows its test runs were never measured, not that there were
// none. rule_match is null on every record this build writes; the rule-match
// layer adds behaviour on v3, not another version.
//
// v4 is additive over v3: the declaration's shape gained status_masked and
// runner_digest, and the execution the same two keys, for the line as it ran;
// both gained rules_version, which lists decided them (shape.RulesVersion). Whether a build or test runner's exit status was hidden by
// the rest of its line is a v4 measurement: a v3 record of `make test |
// tail` says nothing about it, which a reader must take as "not measured",
// not as "not hidden". Nothing else changed, and v1 to v3 records still read.
//
// v5 is additive over v4: every call record names the binary that wrote it
// (writer), a terminal names the invocation that wrote it (phase), and the
// declaration and execution carry the keys of DeclarationV5 and ExecutionV5
// -- the command, paths, targets, git state, failure class and the rest --
// every one of them always present. Null means not applicable or not looked
// at, which is what each of them is until the change that fills it lands; a
// v4 record read back has none of them, and a reader takes that as "not
// measured" as it does for every earlier step. v5 also has one record that
// is not additive, the flush_failed marker: see FlushFailedSchemaVersion.
//
// v6 is additive over v5: the declaration's shape and the execution gained
// runner_kind, the list the runner runner_digest digests is on (D9), set
// exactly when runner_digest is. It is a version of its own rather than a
// v5 key because v5 records are already on disk without it (D11): a v5
// record's runner digest has no recorded kind, which a reader must not take
// for either list. The terminal gained nothing and is written at 6 with its
// declaration.
//
// v7 is additive over v6: the declaration gained store_named, whether the
// call's raw input named rashomon's store root (D173b), kept at every record
// scope so the trust drop reads it where the scope withholds the paths. A
// version of its own for the same reason (D11): a v6 record has no such key,
// and a reader reads it false. The execution and the terminal gained nothing
// and are written at 7 with their declaration.
//
// SchemaVersion is the version of a call's ordinary records: its
// declaration, the terminal that actually closes it, and its execution,
// KindVersion's entry for each. Coverage and gap records are written at
// SessionSchemaVersion instead.
const SchemaVersion = CallSchemaVersion

// FlushFailedSchemaVersion is the version the flush_failed marker is written
// at (REC-34). A literal, not SchemaVersion: the two are independent facts
// (one about ordinary call records, one about a single marker type) and
// tying them would move this one the moment someone edited the other for an
// unrelated reason.
//
// A flush_failed terminal is not additive the way v2 to v4 were: no field
// was added to the TYPE, and nothing a v4 record carries changed meaning.
// What changed is which terminals a reader may treat as closing a
// declaration. Terminal.Reason can now be ReasonFlushFailed, marking a call
// whose records a failed flush left possibly not durable -- and such a
// terminal must NOT close the declaration it names, or a possibly-lost call
// reads as a finished one. A v4 reader has no FlushFailed check; it closes
// on any terminal. Bumping ITS OWN version is what keeps a v4-or-earlier
// reader from reading one of these at all: that reader's Accepts runs before
// the type switch (classifyRunLine), so it skips the marker's line instead
// of half-understanding it.
//
// The marker was at 5 alone until the call records' v5 keys landed (P-11):
// SchemaVersion stayed at 4 while an ordinary record had none of a v5
// record's content, because stamping it 5 anyway would have been the lie
// kinds.go's CallSchemaVersion warns against -- a record claiming a version
// none of whose keys it carries -- and would have cost every 1.1.x reader
// every call record in a run the moment one flush failed. With DeclarationV5,
// ExecutionV5 and TerminalV5 embedded, every call record carries the v5 keys
// and was written at 5 too, at 6 once runner_kind landed and at 7 with
// store_named (D11); the
// marker stays at 5. A 1.1.x reader skips the calls and reads the session
// unverified, as it did for v4, and coverage and gaps stay at 3 so
// it still lists every session.
//
// A new record type instead of a version was rejected: Terminal is the type
// every other call-record consumer already walks, and moving the marker out
// of it would have meant auditing every one of them for an assumption that
// a terminal is the only thing that closes a call, rather than teaching the
// one function (AcceptsKind) that already gates what a reader understands.
const FlushFailedSchemaVersion = 5

// SessionSchemaVersion is the version of the records that describe a session
// rather than one call: coverage and gaps. Neither has changed since v3, so
// they stay at 3 when the call records move on. A released reader accepts
// only the versions it knows and skips the rest; a coverage record it skips
// reads as `start recorded: no` and `probe_absent`, a gap it skips as no
// gap at all. Stamping them with the call records' version would lose the
// whole session to an older binary for a change that is only in the calls.
//
// A terminal is a call record: it goes with its declaration, whose version
// it carries. A readable terminal beside an unreadable declaration reads as
// a declaration that never landed, which is a lock timeout to the reader.
const SessionSchemaVersion = 3

// Record type discriminators.
const (
	TypeDeclaration = "declaration"
	TypeExecution   = "execution"
	TypeTerminal    = "terminal"
	TypeCoverage    = "coverage"
	TypeGap         = "gap"
	// TypeForgottenHost is a store-level forget: see ForgottenHostRecord.
	TypeForgottenHost = "forgotten_host"
)

// Files within a run directory, and at the store root.
const (
	FileRecords  = "records.ndjson"
	FileSpill    = "spill.ndjson"
	FileCoverage = "coverage.ndjson"
	FileProbe    = "probe"
	FileSeq      = "seq"
	FileGaps     = "gaps.ndjson"
	// FileForgottenHosts is at the store root: one ForgottenHostRecord per
	// `forget --host`, whatever it removed.
	FileForgottenHosts = "forgotten-hosts.ndjson"
	// FileLockWait holds, as decimal milliseconds, the longest any one hook
	// invocation of the run waited for store locks (perf-7). It is a note
	// about the store, like probe and seq, and not a record.
	FileLockWait = "lockwait"
	// FileResolvedCap is at the store root: the size cap the last hook
	// invocation resolved, written so status can show the value eviction
	// actually runs with instead of reading EnvCapBytes in its own,
	// possibly different, environment (hld-7).
	FileResolvedCap = "resolved-cap"
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
	// wrote down, whether or not the call ran and whether or not anything was
	// ever reached. That is the entire point -- the report lists them as what
	// was named, and says plainly that nothing here observed whether any was
	// reached.
	//
	// They are separate lists because an ssh host is a different kind of
	// destination from an HTTPS one, and the report's family and novelty views
	// treat it so. Merging them would lose that distinction downstream.
	//
	// Both are sorted and de-duplicated by the extractor so two runs of one
	// command produce byte-identical records. Null when the call named none;
	// never an empty array, because "we looked and found none" and "we did not
	// look" must stay distinguishable for a v1 record read back.
	Hosts    []string `json:"hosts"`
	SSHHosts []string `json:"ssh_hosts"`

	// CWDDigest is the keyed digest of the directory its command starts in
	// (v3), from hook.Locate, its one producer, under the per-install key, so
	// two calls' directories compare equal or not and nothing else. One of
	// three: the leading cd steps shape.LeadingDirectory reads folded onto the
	// PreToolUse payload's cwd, `~` expanded with the hook's own home
	// (shape.CWDDigest; the payload's cwd itself where there are none); for
	// leading steps the program search steps over and LeadingDirectory does not
	// read, shape.UnreadCWDDigest of the payload's cwd and those steps, which
	// equals no read directory's digest; or "", below. Measured on Claude Code
	// 2.1.280, the payload's cwd follows the shell: after `cd sub && pwd` the
	// next call's reads .../sub, so the cwd is where the shell was before the
	// line ran, and where its leading cds lead is where the rest of the line
	// ran. Two runs of one command line in two directories are two different
	// runs, and the test-bending pairs compare this as well as the shape
	// digest.
	//
	// A digest, for the readers keyed on one: the test-bending pairs compare
	// it, and it names where a line's leading cds lead, which no path field
	// does. The declaration carries the payload's cwd itself as well (CWD,
	// with Project and FilePath beside it): a path is less than content, the
	// store is local and nothing in this program sends it anywhere, and forget
	// and purge remove what it holds. Empty string when that directory is not
	// known: no payload cwd to resolve a relative step against or to key
	// unread steps on, no home to expand `~` with, or a read fold past
	// shape.MaxLeadingCd steps.
	CWDDigest string `json:"cwd_digest"`

	// FileLabel is what the file this call named looks like (v3), from the
	// label layer (internal/hook's fileLabel, internal/shape's Label).
	//
	// No omitempty, deliberately. The key is always present, null when the
	// tool names no file, because the record allowlist is an exact set in
	// both directions -- a key that appears only sometimes is a key the
	// allowlist cannot describe, and "sometimes present" is the shape a
	// reader cannot tell from "absent because nothing looked".
	//
	// Three states, all different: null means the tool names no file, so
	// nobody looked; "none" means a real path matched no row; "unknown" means
	// a path could not be read at all. The path itself is never stored here.
	FileLabel *string `json:"file_label"`

	// StoreNamed is one bit: whether the call's raw input names rashomon's
	// store root, its slashes plain or escaped (internal/hook's rootNamed,
	// D173b). The root is compared and dropped, never stored. Unlike the
	// paths, it is kept at every record scope, so a record whose scope or
	// caps withhold the paths still says the call named the store, and
	// verify.StoreWritten reads it there. A read that names the root sets
	// it too. Schema 7, a version of its own (D11): a record before it has
	// no such key and reads false, as a call that did not name the root.
	StoreNamed bool `json:"store_named"`

	// HostSource says where each entry of Hosts came from, positionally (v3).
	// "structured" is a host read from a field whose value is a URL, so the
	// call names it by construction; "lexical" is one extracted from command
	// text, where a mention and a destination are not the same thing --
	// `echo see https://example.com` yields the host and reaches nothing.
	//
	// RESERVED AND NOT YET POPULATED. The extraction that would fill it lives
	// in internal/shape and internal/hook, which another branch owns; this
	// declares the shape so that branch adds behaviour rather than a second
	// schema version. Null here means nobody looked, which is not the same as
	// an empty list.
	HostSource []string `json:"host_source"`

	// RuleMatch is the rule-match layer's verdict for this call (v3).
	//
	// ITS SHAPE BELONGS TO THAT LAYER and is deliberately open, so the layer
	// can add fields without a fourth schema version. Null means NOT COMPUTED
	// BY THIS VERSION -- not "no rule matched", which is a result and will be
	// expressed inside the object. A reader that cannot tell those apart
	// reports an absence of rules as an absence of matches.
	RuleMatch map[string]any `json:"rule_match"`

	// RulesVersion is shape.RulesVersion as the writing binary had it (v4):
	// which runner and build lists decided this record's shape.verb_class
	// and shape.status_masked. Records written before and after a list
	// changed are told apart by it, and by nothing else. It names the writer's
	// lists only: the pass words are applied when a report is read, never
	// stored.
	RulesVersion int `json:"rules_version"`

	// DeclarationV5 is the declaration's v5 keys (record_v5.go).
	DeclarationV5
}

// Execution records that a declared call ran, one per PostToolUse invocation.
//
// A declaration with an execution beside it ran. A declaration without one was
// denied, failed, or had its execution go unrecorded, and nothing here claims
// to know which of the three it was.
//
// It carries the join key, the clock and the tool name, and no field that could
// hold any part of a tool response. The PostToolUse payload carries the
// response; internal/hook declares no field for its content, so it is never a
// value in this process, and there is nowhere here for it to be put if it were.
// The one bit taken from it is Backgrounded: whether two keys are present.
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
	// `error: "Exit code N"` (v2). Only the first line is read: a command that
	// printed anything fails with "Exit code N\n<its output>", and the output
	// is never looked at.
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

	// ExecutedDigest is the shape digest of the input as it ACTUALLY RAN (v2).
	//
	// Measured on Claude Code 2.1.258: PreToolUse carries tool_input as the
	// model asked for it and PostToolUse carries it after any hook rewrote it,
	// under the same tool_use_id. Comparing this against the declaration's
	// shape.digest is therefore the only way a rewriting hook is visible at
	// all -- the transcript keeps only the pre-rewrite input, which is exactly
	// the record an agent with shell access could have shaped.
	//
	// A digest and never the input. Both sides are HMAC'd with the same
	// per-install key, so they are comparable to each other and to nothing
	// else; the report can say the executed call differed from the declared one
	// without being able to say how.
	//
	// Empty when the payload carried no tool_input. Empty must NOT compare
	// unequal to a declared digest: an unknown digest counted as a difference
	// would report every such call as rewritten.
	ExecutedDigest string `json:"executed_digest"`

	// RuleMatch is the rule-match layer's verdict for the call as it RAN (v3).
	// Reserved and not yet populated; see the note on Declaration.RuleMatch.
	RuleMatch map[string]any `json:"rule_match"`

	// Backgrounded is true when this record was written while the command was
	// still running in the background (v3): Claude Code moved it there at its
	// timeout, or it was launched with run_in_background (both measured), or
	// on Ctrl+B, recognised only if Claude Code marks it with
	// backgroundTaskId or backgroundedByUser, which was not measured. Its
	// PostToolUse fires then, so Outcome is the launch's, not the command's,
	// and a reader treats the call's ending as unobserved (see
	// report.LinkOutcomeBackgrounded).
	//
	// Read from the presence of two keys of a Bash call's tool_response and
	// nothing else of it; see internal/hook's backgroundResponse. False on
	// every other tool, on a failure event, which means the command ended or
	// its launch failed, and on a record written before v3, which could not
	// tell.
	Backgrounded bool `json:"backgrounded"`

	// StatusMasked and RunnerDigest are the shape's two fields of the same
	// names (v4), derived from the input as it ACTUALLY RAN, by the same
	// Derive call that gives ExecutedDigest. A hook that rewrites the line
	// before it runs -- to pipe test output into `grep | head`, say -- makes
	// the declaration's the wrong line to judge masking on; a reader takes
	// these instead wherever the record saw a tool_input (report.MaskingAsRan).
	// Null when the payload carried no tool_input, and on a tool that is not
	// a shell.
	StatusMasked *string `json:"status_masked"`
	RunnerDigest *string `json:"runner_digest"`
	// RunnerKind is the shape's runner_kind for the line as it ran (v6), from
	// the same Derive, so it is set exactly when RunnerDigest is.
	RunnerKind *string `json:"runner_kind"`

	// RulesVersion is shape.RulesVersion as the writing binary had it (v4):
	// which runner and build lists decided StatusMasked. See Declaration's.
	RulesVersion int `json:"rules_version"`

	// ExecutionV5 is the execution's v5 keys (record_v5.go).
	ExecutionV5
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
	// Phase is PhaseCall or PhasePost, naming which invocation wrote this
	// terminal (v5). It is null for an ordinary closing terminal, which
	// Handler.Close always writes from the hook's own PreToolUse process
	// and so carries no ambiguity worth a field for. It is set on a
	// ReasonFlushFailed marker, where it does: PhaseCall means the
	// DECLARATION this invocation itself wrote may not be durable; PhasePost
	// means the EXECUTION post just appended may not be, while the
	// declaration -- flushed by an earlier, separate hook invocation -- is
	// unaffected. Without it a reader sees a marker and cannot tell which.
	Phase *string `json:"phase"`

	// TerminalV5 is the terminal's other v5 key, its writer.
	TerminalV5
}

// FlushFailed reports a terminal that marks its call's records as possibly
// not durable (ReasonFlushFailed) rather than closing the call. It is not an
// ending: a reader must not let it close a declaration whose own terminal
// never landed, or count it as a declaration the lock dropped.
func (t Terminal) FlushFailed() bool {
	return t.Reason != nil && *t.Reason == ReasonFlushFailed
}

// Coverage states.
const (
	StateVerified   = "verified"
	StateUnverified = "unverified"
)

// Coverage reasons. Fixed codes, never free text, and never derived from input.
const (
	ReasonInternalError = "internal_error"
	// ReasonRecoveredPanic marks an invocation in which the panic barrier
	// recovered a panic: a bug in this program, as distinct from
	// internal_error's ordinary failure (a write the disk refused, a payload
	// that did not parse). It rides the coverage record and, when the panic
	// cost the call's capture, its terminal. A panic recovered inside the
	// file-label derivation costs only the label, which reads unknown, and
	// is named here so the report can tell that unknown from an ordinary one.
	ReasonRecoveredPanic = "recovered_panic"
	ReasonLockTimeout    = "lock_timeout"
	// ReasonPayloadTooLarge marks an invocation whose payload was over the
	// 8 MiB intake cap (hookio.MaxPayloadBytes) and was refused after a
	// bounded read. Nothing broke inside this program, which is why it is
	// not internal_error; the call itself went unrecorded.
	ReasonPayloadTooLarge = "payload_too_large"
	// ReasonFlushFailed marks records a hook appended and then could not
	// flush to stable storage (REC-34): they read back, and a power loss or
	// a crash of the machine could still remove them. It rides a spill
	// terminal for the call, the one record a process can append without
	// waiting on a lock, because coverage appends take theirs.
	ReasonFlushFailed         = "flush_failed"
	ReasonTerminatedBySignal  = "terminated_by_signal"
	ReasonUnterminatedEntry   = "unterminated_entry"
	ReasonHookEntryAbsent     = "hook_entry_absent"
	ReasonHookEntryUnresolved = "hook_entry_unresolved"
	ReasonProbeAbsent         = "probe_absent"
	ReasonProbeUnresolved     = "probe_unresolved"
	// ReasonRecordingPaused marks an invocation that found recording paused
	// (Part 2). It is not a failure -- nothing here is broken -- but reason is
	// the only field a coverage record has to say "no, and here is why", and a
	// second field just for this would let a reader check State and miss it.
	// checkPaused in cmd/rashomon/main.go is the only writer.
	ReasonRecordingPaused = "recording_paused"
	// ReasonRecordOversize marks a call whose record was over a size limit:
	// a field over its cap was withheld from it, or the whole record was
	// over MaxRecordBytes and refused (REC-31, REC-52). Nothing broke, which
	// is why it is not internal_error; what the record holds is incomplete.
	ReasonRecordOversize = "record_oversize"
	// ReasonPauseStateUnreadable marks an invocation that found the pause
	// marker present and could not read it: EACCES on the file itself, or
	// any other error that is not the file simply not being there. It is
	// recorded instead of ReasonRecordingPaused because the call really was
	// paused, but not on purpose the way an ordinary `rashomon pause` is --
	// a reader needs the two distinguishable rather than folded into one.
	// checkPaused is the only writer.
	ReasonPauseStateUnreadable = "pause_state_unreadable"
	// ReasonRecordScopeUnresolved marks a session start that could not set
	// the record-scope marker from consent.json: consent.json could not be
	// read, or the marker could not be written. The marker is then absent,
	// so the session's calls record the v4 shape and withhold every content
	// field with no_consent, whatever the consent on file says; what the
	// records hold may be less than the user agreed to. The SessionStart
	// probe is the only writer.
	ReasonRecordScopeUnresolved = "record_scope_unresolved"
)

// Reasons lists every coverage reason code a record can carry. It exists so
// that the store schema can be checked against the vocabulary the code writes
// rather than against a copy of it that drifts.
func Reasons() []string {
	return []string{
		ReasonInternalError,
		ReasonRecoveredPanic,
		ReasonLockTimeout,
		ReasonPayloadTooLarge,
		ReasonFlushFailed,
		ReasonTerminatedBySignal,
		ReasonUnterminatedEntry,
		ReasonHookEntryAbsent,
		ReasonHookEntryUnresolved,
		ReasonProbeAbsent,
		ReasonProbeUnresolved,
		ReasonRecordingPaused,
		ReasonRecordOversize,
		ReasonPauseStateUnreadable,
		ReasonRecordScopeUnresolved,
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
//
// EntryPresentSettings and EntryPresentPlugin are two different questions with
// two different kinds of answer, not two spellings of one "present". A
// settings entry is verified by a fresh read of a file Claude Code's own file
// watcher keeps current; a plugin's is verified by this process itself running
// as that plugin's hook binary, because there is no documented guarantee that
// enabling a plugin reaches an already-running session before its next call.
// Collapsing them into one value would let a coverage record claim the
// stronger kind of evidence while holding the weaker kind -- see
// internal/hook/coverage.go's Resolve for where each is decided.
//
// EntryPresent is never written by this code -- Resolve always picks one of
// the two origin-specific values above, or Absent, or Unknown -- but it is
// not retired, because every binary shipped before Part 1 wrote it, into
// records that are schema version 2 and are not going anywhere. Removing it
// from this list would make the enum describe a narrower store than the one
// actually on disk: a real consumer validating a real store against
// docs/store-schema.json would fail on records this program itself wrote,
// which is the exact failure TestStoreSchemaMatchesTheAllowlists exists to
// catch in the other direction. A reader that meets it should treat it as
// "present, origin unknown" -- true of both origins before this document
// existed to tell them apart -- and it costs nothing to keep meaning that:
// report.go copies HookEntry through unexamined for display, and nothing
// re-derives a stored record's own State from HookEntry at read time (that
// decision was made once, by whichever binary wrote the record, and is
// carried in State itself) -- so a legacy Present value was frozen as
// verified the day it was written and renders that way today, unchanged.
// TestLegacyPresentRendersVerified pins that rather than leaving it
// incidental.
const (
	EntryPresentSettings = "present_settings"
	EntryPresentPlugin   = "present_plugin"
	EntryPresent         = "present"
	EntryAbsent          = "absent"
	EntryUnknown         = "unknown"

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

	// CWD is the working directory the session ran in (v2).
	//
	// It is here because the novelty baseline is per PROJECT, and the project
	// is derived from this path -- so without it the report would have to guess
	// at the project from the transcript path's mangled form, or key every
	// session on this machine to one baseline.
	//
	// It is metadata of the same class as transcript_path, which this store has
	// always kept verbatim: a path the client supplied, used as a join key. It
	// is not content, and note what it still cannot hold -- Coverage has no
	// numeric field and gains none here, so the rule that a run whose
	// instrumentation failed cannot report a count is untouched.
	//
	// Empty on a record written by a path that had no cwd to report.
	CWD string `json:"cwd"`
}

// ForgottenHostRecord is a host-scoped forget, kept at the store level.
//
// SEPARATE FROM THE GAPS ON PURPOSE. A gap says "records were removed here",
// and `forget --host` writes one only for a run it actually removed records
// from. That left a hole the nono adapter made visible (#45): a host that only
// the sandbox's trail held -- or one whose every naming call had already left
// with an earlier forget of another host -- could not be forgotten at all, and
// the next report listed it again. This record is written on every
// `forget --host`, so the report's predicate covers every source it reads, and
// the gaps keep meaning only what they always meant.
//
// It carries the same keyed digest a gap does and never the name, for the
// reason set out on Gap.HostDigest.
//
// STAMPED AT SessionSchemaVersion, decided rather than inherited. It is a
// store-level record, like a gap, and no released reader opens this file: a
// version of its own would version a record no older binary can meet. Its
// key set is closed by forgottenHostKeys in the acceptance tests.
type ForgottenHostRecord struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	RecordedAtMS  int64  `json:"recorded_at_unix_ms"`
	HostDigest    string `json:"host_digest"`
}

// Gap reasons.
const (
	GapForget     = "forget"
	GapForgetHost = "forget_host"
	GapSizeCap    = "size_cap"
	// GapPaused marks a window pause and resume opened together, rather than
	// one either removed anything from. Every other reason above describes
	// records that existed and left; this one describes records that were
	// never made, on purpose -- RemovedRecords is legitimately 0 for it. It is
	// written twice per pause/resume cycle: once by pause, zero-width, as
	// evidence the window began even if resume never follows; once by resume,
	// spanning the whole window, which is the one a reader actually wants. See
	// cmd/rashomon's cmdPause and cmdResume.
	GapPaused = "paused"
)

// Gap records that records left the store, or that a deliberate pause meant
// none were made, and why either way. Before Part 2 it was only the first of
// those, and "the only way records leave" was a complete description; a
// paused window is the one case a Gap records where nothing left because
// nothing was ever there, and reusing this type rather than inventing a
// second one keeps "is there a hole here, and why" answerable from one place.
// A deletion with no gap record is a silent deletion, and a store that can be
// silently edited is worth nothing as evidence.
type Gap struct {
	Type           string `json:"type"`
	SchemaVersion  int    `json:"schema_version"`
	RecordedAtMS   int64  `json:"recorded_at_unix_ms"`
	SessionID      string `json:"session_id"`
	Reason         string `json:"reason"`
	FromUnixMS     int64  `json:"from_unix_ms"`
	ToUnixMS       int64  `json:"to_unix_ms"`
	RemovedRecords int    `json:"removed_records"`

	// HostDigest identifies the destination a host-scoped forget removed (v2),
	// as an HMAC under the per-install key. Empty for a window forget, which
	// removes by time and names no host.
	//
	// A DIGEST and not the hostname, and the reason is a conflict this field
	// had to resolve. The report needs to keep suppressing the host, because
	// nono's own trail is not ours to delete from and the host would otherwise
	// reappear in the next report under the sandbox heading -- the forget
	// reading as undone. But `forget --host` also has to make the hostname
	// LEAVE the store's bytes, which is the whole point of forgetting it.
	// Writing the name here would have satisfied the first requirement by
	// breaking the second.
	//
	// Keyed with the same per-install HMAC key the shape digests use, so the
	// report can recompute it from an observed host while a reader of the store
	// -- or of a copy of it -- cannot turn it back into a name, and cannot test
	// a guess without the key.
	HostDigest string `json:"host_digest"`
}
