package store

// The schema-5 field list (technical spec 1.2.3 to 1.2.10; archive 10.2.2 to
// 10.2.14, 10.2.17; BREADTH's two kinds at archive 60).
//
// Every v5 key is always present on a v5 record: null means not applicable
// or not looked at, [] means looked and found none, and a reader that meets
// either knows which. That rule is why these are pointer and slice fields
// without omitempty. The call records embed DeclarationV5, ExecutionV5 and
// TerminalV5 and are written at CallSchemaVersion; each key stays null until
// the change that fills it lands, which is "not looked at", the truth. The
// kinds introduced at 1.2.0 are frozen here as types their writers emit when
// they land.
//
// Where the spec names a key without its shape, the field says so with
// "P-07 proposal" and the PR body lists it; everything else is the spec's.

// Cut records where a stored string was shortened to its head and tail
// windows and how many bytes went between them.
type Cut struct {
	Offset       int `json:"offset"`
	RemovedBytes int `json:"removed_bytes"`
}

// MaskSpan is one masked value in a stored string: the rule that masked it
// and the byte offsets of the marker in the stored text.
type MaskSpan struct {
	Rule  string `json:"rule"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// DeclarationV5 are the keys a v5 declaration carries beyond v4 (archive
// 10.2.2).
type DeclarationV5 struct {
	Writer string `json:"writer"`
	// Command is the command as typed, masked, up to 64 KiB whole or a head
	// and tail (1.2.3 command capture); CommandWithheld says why it is null:
	// no_consent | switched_off | masker_error | oversize | forgotten.
	Command         *string    `json:"command"`
	CommandBytes    *int       `json:"command_bytes"`
	CommandCut      *Cut       `json:"command_cut"`
	Masked          []MaskSpan `json:"masked"`
	CommandWithheld *string    `json:"command_withheld"`
	CWD             *string    `json:"cwd"`
	// Project is baseline.Key of the start directory, the project key every
	// layer shares (Contracts and names).
	Project  *string `json:"project"`
	FilePath *string `json:"file_path"`
	// WrittenPaths are the lexical write targets of a Bash call that may
	// write, at most 64 entries and 16 KiB, withheld whole over either bound
	// (null, with WrittenPathsWithheld "oversize"); WrittenPathsComplete is
	// true only when every write-capable segment yielded a target, and false
	// when the list is withheld (1.2.3; 1.2.11).
	WrittenPaths         []string `json:"written_paths"`
	WrittenPathsComplete *bool    `json:"written_paths_complete"`
	Target               *Target  `json:"target"`
	// EditMarksBefore are the counts per closed mark id in the file a Write
	// overwrites, taken at PreToolUse from the one pre-read (1.2.5, Wave B;
	// RES-37); null for every other tool.
	EditMarksBefore map[string]int `json:"edit_marks_before"`

	// The *_withheld keys say why their field is null when it is null by
	// rule rather than by "not applicable": "oversize" over a cap of 1.2.11
	// ("Over cap: null plus `<field>_withheld: "oversize"`"), or
	// "switched_off" under a record.* switch of config.json (archive
	// 10.2.17: "withheld fields carry `..._withheld: "switched_off"`, so
	// 'not recorded by choice' never reads as 'nothing happened'"). Null
	// when the field is present or simply not applicable. P-07 proposal
	// (owner K, RECORD), spelled as 1.2.11 spells it, beside the
	// command_withheld 1.2.3 already lists: one key per field a cap or a
	// switch can null, on every v5 type, as TestDEL22g lists them; a whole
	// kind switched off writes no record, and the session's
	// scopes.switches says so. The v4 call record's nullable capped fields
	// (prompt_id, agent_id, agent_type, hosts, ssh_hosts) take their keys
	// here, since the declaration embeds this struct beside them. Deferred, with the
	// reason in TestDEL22g: the v4 call record's non-nullable capped fields
	// (tool_use_id, transcript_path, permission_mode, tool_name,
	// shape.program), whose null half needs the v4 structs retyped,
	// and outcome_join's tool_use_id, BREADTH's join key.
	WrittenPathsWithheld    *string `json:"written_paths_withheld"`     // oversize | switched_off (paths)
	FilePathWithheld        *string `json:"file_path_withheld"`         // oversize | switched_off (paths)
	CWDWithheld             *string `json:"cwd_withheld"`               // oversize | switched_off (paths)
	ProjectWithheld         *string `json:"project_withheld"`           // oversize
	TargetWithheld          *string `json:"target_withheld"`            // switched_off (targets)
	EditMarksBeforeWithheld *string `json:"edit_marks_before_withheld"` // switched_off (edit_marks)
	PromptIDWithheld        *string `json:"prompt_id_withheld"`         // oversize
	AgentIDWithheld         *string `json:"agent_id_withheld"`          // oversize
	AgentTypeWithheld       *string `json:"agent_type_withheld"`        // oversize
	HostsWithheld           *string `json:"hosts_withheld"`             // oversize (over 32 entries)
	SSHHostsWithheld        *string `json:"ssh_hosts_withheld"`         // oversize (over 32 entries)

	// BREADTH's call-record keys, "Additive, in the schema 5 bump" (tech
	// 6.2.1): which harness declared the call, the harness's own tool name
	// when it differs from the canonical one (^[A-Za-z0-9_:.\-]{1,128}$,
	// else null), and whether the declaration came from a pre or a post
	// hook (pre | post).
	HarnessID       *string `json:"harness_id"`
	HarnessToolName *string `json:"harness_tool_name"`
	DeclaredVia     *string `json:"declared_via"`
}

// ExecutionV5 are the keys a v5 execution carries beyond v4 (archive 10.2.2).
type ExecutionV5 struct {
	Writer string `json:"writer"`
	// ExecutedCommand is the command as it ran, when it differs from the
	// declaration's (ExecutedCommandReason: differs | declaration_not_in_tail).
	ExecutedCommand       *string         `json:"executed_command"`
	ExecutedCommandReason *string         `json:"executed_command_reason"`
	ExecutedCommandBytes  *int            `json:"executed_command_bytes"`
	ExecutedCommandCut    *Cut            `json:"executed_command_cut"`
	ExecutedMasked        []MaskSpan      `json:"executed_masked"`
	Failure               *Failure        `json:"failure"`
	OutputEvidence        *OutputEvidence `json:"output_evidence"`
	EditMarks             *EditMarks      `json:"edit_marks"`
	// HeadAfter is the git state after a HEAD-moving vcs call (1.2.6).
	HeadAfter *GitState `json:"head_after"`
	// GateOutput is VERIFY's classification of a gate-family call's output
	// (2.2.3, 2.2.10): the kind, the parser and, for unread, the reason.
	GateOutput *GateOutput `json:"gate_output"`
	// CredMech names the credential mechanism a call used (a profile, a
	// role, an env var name, a vault path; never a value), read by
	// cred-spray, whose refute rule needs it on the later success as much as
	// on the failure. Name and type are frozen here (Contracts and names);
	// its closed values and how it is filled land with P-14. P-07 proposal:
	// it sits on every execution rather than under failure.
	CredMech *string `json:"credmech"`

	// The *_withheld keys, as DeclarationV5's (P-07 proposal, owner K):
	// ExecutedCommandWithheld takes command_withheld's values (no_consent |
	// switched_off | masker_error | oversize | forgotten), the rest
	// switched_off under failure_classes, output_evidence, edit_marks and
	// git.
	ExecutedCommandWithheld *string `json:"executed_command_withheld"`
	FailureWithheld         *string `json:"failure_withheld"`
	OutputEvidenceWithheld  *string `json:"output_evidence_withheld"`
	EditMarksWithheld       *string `json:"edit_marks_withheld"`
	HeadAfterWithheld       *string `json:"head_after_withheld"`

	// BREADTH's call-record keys (tech 6.2.1), as on DeclarationV5, and
	// the execution's own: where the outcome came from (claude_post |
	// claude_failure | cursor_tool_output | cursor_failure |
	// codex_mcp_is_error | codex_rollout_join | none), the failure type
	// (error | timeout | permission_denied, else null) and an MCP call's
	// server source (plugin | sdk | user | project | local | managed |
	// claudeai | other, else null).
	HarnessID       *string `json:"harness_id"`
	HarnessToolName *string `json:"harness_tool_name"`
	DeclaredVia     *string `json:"declared_via"`
	OutcomeSource   *string `json:"outcome_source"`
	FailureType     *string `json:"failure_type"`
	MCPServerSource *string `json:"mcp_server_source"`
}

// TerminalV5 are the keys a v5 terminal carries beyond v4: only the writer
// (1.2.3: "Terminal gains only schema_version and writer").
type TerminalV5 struct {
	Writer string `json:"writer"`
}

// Target is where a failing call was pointed, from its arguments and inline
// env prefixes only, never the hook's environment (1.2.5; archive 10.2.6).
// Every value passed the charset, every masker rule and the entropy test;
// Source says where each present key came from, and Dropped lists the keys
// whose value failed a check, the value itself never stored.
type Target struct {
	AWSProfile        *string           `json:"aws_profile"`
	AWSRegion         *string           `json:"aws_region"`
	AWSAccount        *string           `json:"aws_account"`
	EndpointHost      *string           `json:"endpoint_host"`
	S3Bucket          *string           `json:"s3_bucket"`
	KubeContext       *string           `json:"kube_context"`
	KubeNamespace     *string           `json:"kube_namespace"`
	GCPProject        *string           `json:"gcp_project"`
	AzureSubscription *string           `json:"azure_subscription"`
	Source            map[string]string `json:"source"` // key -> arg | env_prefix | subcommand
	Dropped           []string          `json:"dropped"`
}

// Failure is the failure class of a PostToolUseFailure (1.2.5), computed in
// memory from text that is never stored.
type Failure struct {
	Origin            string `json:"origin"` // exit | no_exit_line | timeout | interrupt | file_tool | mcp | web | agent | other_tool
	Class             string `json:"class"`
	Source            string `json:"source"` // program_table | text | exit_only | none
	CountsAsFailure   bool   `json:"counts_as_failure"`
	CountsForLearning bool   `json:"counts_for_learning"`
	ClassTableVersion int    `json:"class_table_version"`
	ErrorBytes        int    `json:"error_bytes"`
}

// OutputEvidence is what one output scan found in a gate run (Wave B,
// RES-38); a masked run verifies only when success_line && !fail_line.
type OutputEvidence struct {
	Family       string `json:"family"`
	SuccessLine  bool   `json:"success_line"`
	FailLine     bool   `json:"fail_line"`
	ScannedBytes int    `json:"scanned_bytes"`
	Version      int    `json:"version"`
}

// EditMarks are the deltas per closed integrity-mark id in an edit (archive
// 10.2.8), from the shared 1 MiB pre-read (RES-37): the same counts
// VERIFY's integrity checks read, keyed by mark id so that a check can name
// the marker it moved on.
type EditMarks struct {
	Added   map[string]int `json:"added"`
	Removed map[string]int `json:"removed"`
	Basis   string         `json:"basis"` // edit_delta | write_vs_before | write_before_unread | notebook_added_only
	Version int            `json:"version"`
}

// GateOutput is the output classification of a gate-family call (2.2.3).
type GateOutput struct {
	Kind   string  `json:"kind"` // summary_pass | summary_fail | fail_line | empty_run | no_summary | unattributable | unread
	Parser string  `json:"parser"`
	Reason *string `json:"reason"` // unread: budget | too_large | decode | verify_off
}

// GitState is the repository state read without running git (1.2.6).
type GitState struct {
	Root     *string `json:"root"`
	Head     *string `json:"head"`   // 40 or 64 hex
	Branch   *string `json:"branch"` // null when detached
	Detached bool    `json:"detached"`
	State    string  `json:"state"` // ok | no_repo | unborn | unreadable
	// RootWithheld: oversize (4,096 B, 1.2.11) | switched_off (paths).
	// A whole git state switched off is its parent's git_withheld.
	RootWithheld *string `json:"root_withheld"`
}

// EventHeader is what every record kind introduced at 1.2.0 starts with.
// Critical is false on all of them: an older reader skips the record and
// the session still reads (1.2.2).
type EventHeader struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	Critical      bool   `json:"critical"`
	Seq           *int64 `json:"seq"`
	RecordedAtMS  int64  `json:"recorded_at_unix_ms"`
	SessionID     string `json:"session_id"`
}

// TurnEnd is written by recap on every Stop and StopFailure before it prints
// (1.2.7; archive 10.2.10).
type TurnEnd struct {
	EventHeader
	PromptID        *string         `json:"prompt_id"`
	Event           string          `json:"event"` // stop | stop_failure
	StopHookActive  *bool           `json:"stop_hook_active"`
	APIError        *string         `json:"api_error"`
	Model           *string         `json:"model"`
	ModelSource     string          `json:"model_source"` // transcript | model_switch | session_start | unknown
	ClaudeCodeVer   *string         `json:"claude_code_version"`
	Effort          *string         `json:"effort"`
	PermissionMode  *string         `json:"permission_mode"`
	Git             *GitState       `json:"git"`
	BackgroundTasks BackgroundTasks `json:"background_tasks"`
	Calls           TurnCalls       `json:"calls"`
	ClaimClass      string          `json:"claim_class"` // none | done | pass | question | failure_reported | unknown
	ClaimWords      []string        `json:"claim_words"`
	GateState       *string         `json:"gate_state"`
	GateVersion     *int            `json:"gate_version"`
	Cost            *TurnCostBody   `json:"cost"`
	RecapSpoke      bool            `json:"recap_spoke"`
	// StoodDown lists the budget reasons of the phases that stood down in
	// this recap (2.2.4 fallback; 8.2.3). P-07 proposal: the key's name.
	StoodDown []string `json:"stood_down"`

	// The *_withheld keys, as DeclarationV5's (P-07 proposal, owner K).
	PromptIDWithheld       *string `json:"prompt_id_withheld"`       // oversize
	PermissionModeWithheld *string `json:"permission_mode_withheld"` // oversize
	GitWithheld            *string `json:"git_withheld"`             // switched_off (git)
	CostWithheld           *string `json:"cost_withheld"`            // switched_off (cost)
}

// BackgroundTasks is the Stop payload's background registry as recorded
// (archive 10.2.10): Present false means the arrays were absent, which is
// unknown, not idle.
type BackgroundTasks struct {
	Present bool             `json:"present"`
	Tasks   []BackgroundTask `json:"tasks"` // at most 16
	Crons   *int             `json:"crons"` // count only; the cron prompt is never stored
}

// BackgroundTask is one registry entry in its closed shape: Claude Code's id,
// type and status, whether the shape read it as a runner, and its command,
// masked and at most 1,000 characters as Claude Code caps it (1.2.11);
// CommandWithheld says why the command is null, as DeclarationV5's does
// (P-07 proposal, owner K).
type BackgroundTask struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	Runner          bool    `json:"runner"`
	Command         *string `json:"command"`
	CommandWithheld *string `json:"command_withheld"` // oversize | switched_off (commands) | masker_error
}

// TurnCalls counts the turn's calls.
type TurnCalls struct {
	Declared    int `json:"declared"`
	Executed    int `json:"executed"`
	Failed      int `json:"failed"`
	Interrupted int `json:"interrupted"`
	Main        int `json:"main"`
	Subagent    int `json:"subagent"`
}

// TurnCostBody is the turn's cost (1.2.7; archive 10.2.10), provisional on
// turn_end and final on turn_cost.
type TurnCostBody struct {
	State               string   `json:"state"` // provisional | final | unread | no_transcript
	ModelTurnsMain      int      `json:"model_turns_main"`
	ModelTurnsSubagents int      `json:"model_turns_subagents"`
	InputTokens         int64    `json:"input_tokens"`
	OutputTokens        int64    `json:"output_tokens"`
	CacheReadTokens     int64    `json:"cache_read_tokens"`
	CacheWriteTokens    int64    `json:"cache_write_tokens"`
	USDNano             int64    `json:"usd_nano"`
	UnpricedResponses   int      `json:"unpriced_responses"`
	PriceTable          string   `json:"price_table"`
	Models              []string `json:"models"`
}

// TurnCost finalises a turn's cost once the next prompt appears (1.2.7).
// P-07 proposal: the cost fields nest under `cost`, as turn_end carries them,
// so one reader serves both records.
type TurnCost struct {
	EventHeader
	PromptID         *string      `json:"prompt_id"`
	Cost             TurnCostBody `json:"cost"`
	PromptIDWithheld *string      `json:"prompt_id_withheld"` // oversize
}

// Prompt is written at UserPromptSubmit (1.2.8; archive 10.2.11); it carries
// no prompt text.
type Prompt struct {
	EventHeader
	PromptID          *string   `json:"prompt_id"`
	PromptSource      string    `json:"prompt_source"` // typed_presumed | scheduled | background_report | cross_session
	PromptClass       *string   `json:"prompt_class"`
	PromptBytes       int       `json:"prompt_bytes"`
	PreviousPromptID  *string   `json:"previous_prompt_id"`
	PreviousTurnState string    `json:"previous_turn_state"` // ended | ended_api_error | no_stop | none
	Git               *GitState `json:"git"`
	PermissionMode    *string   `json:"permission_mode"`

	// The *_withheld keys, as DeclarationV5's (P-07 proposal, owner K).
	// prompt_class is off by default until D-REC-3 (1.2.12), so on a
	// fresh install prompt_class_withheld reads switched_off.
	PromptIDWithheld         *string `json:"prompt_id_withheld"`          // oversize
	PreviousPromptIDWithheld *string `json:"previous_prompt_id_withheld"` // oversize
	PromptClassWithheld      *string `json:"prompt_class_withheld"`       // switched_off (prompt_class)
	GitWithheld              *string `json:"git_withheld"`                // switched_off (git)
	PermissionModeWithheld   *string `json:"permission_mode_withheld"`    // oversize
}

// Human is a human's intervention in the session (1.2.8; archive 10.2.11);
// it carries no person identifier.
type Human struct {
	EventHeader
	PromptID  *string  `json:"prompt_id"`
	AgentID   *string  `json:"agent_id"`
	Kind      string   `json:"kind"`
	Source    string   `json:"source"`
	ToolName  *string  `json:"tool_name"`
	ToolUseID *string  `json:"tool_use_id"`
	JoinedBy  string   `json:"joined_by"` // tool_use_id | digest | none | ambiguous
	Reason    *string  `json:"reason"`    // masked, 128 B
	Evidence  []string `json:"evidence"`

	// The *_withheld keys, all oversize (1.2.11), as DeclarationV5's
	// (P-07 proposal, owner K).
	PromptIDWithheld  *string `json:"prompt_id_withheld"`
	AgentIDWithheld   *string `json:"agent_id_withheld"`
	ToolNameWithheld  *string `json:"tool_name_withheld"`
	ToolUseIDWithheld *string `json:"tool_use_id_withheld"`
}

// RecordScopes is the consent scope and the switches in force when the
// session started (archive 10.2.12, 10.2.17): the spec addresses
// scopes.record_scope; P-07 proposal: the record.* switches of config.json
// beside it, keyed by their config name.
type RecordScopes struct {
	RecordScope string          `json:"record_scope"` // v4 | v5
	Switches    map[string]bool `json:"switches"`
}

// ResumeFields are what a resumed session's SessionStart carries (archive
// 10.2.12; hooks.md 1142-1145); null on every other source.
type ResumeFields struct {
	SecondsSinceLastResponse *int64 `json:"seconds_since_last_response"`
	ContextTokens            *int64 `json:"context_tokens"`
}

// Session is written by probe start at every SessionStart source (1.2.8;
// archive 10.2.12). It carries exactly one claude_code_version, with its
// source, which is the value the mirror verification is checked against
// (7.2.5).
type Session struct {
	EventHeader
	Source                  string            `json:"source"`
	Model                   *string           `json:"model"`
	AgentType               *string           `json:"agent_type"`
	Harness                 string            `json:"harness"`
	HarnessEvidence         []string          `json:"harness_evidence"`
	Writer                  string            `json:"writer"`
	RulesVersion            int               `json:"rules_version"`
	ClassTableVersion       int               `json:"class_table_version"`
	MaskerVersion           int               `json:"masker_version"`
	Project                 *string           `json:"project"`
	Git                     *GitState         `json:"git"`
	Scopes                  RecordScopes      `json:"scopes"`
	Entries                 map[string]string `json:"entries"` // event -> present_settings | present_plugin | absent | unknown
	SteerState              string            `json:"steer_state"`
	ClaudeCodeVersion       *string           `json:"claude_code_version"`
	ClaudeCodeVersionSource string            `json:"claude_code_version_source"` // transcript | unknown
	CouldNotSee             []string          `json:"could_not_see"`
	Resume                  *ResumeFields     `json:"resume"`

	// The *_withheld keys, as DeclarationV5's (P-07 proposal, owner K).
	AgentTypeWithheld *string `json:"agent_type_withheld"` // oversize
	ProjectWithheld   *string `json:"project_withheld"`    // oversize
	GitWithheld       *string `json:"git_withheld"`        // switched_off (git)
}

// Subagent is written at SubagentStart and SubagentStop (1.2.10; archive
// 10.2.14): the stop phase adds the claim class, stop_hook_active and whether
// background work was present; an internal agent (empty agent_type) is
// recorded with Internal true.
type Subagent struct {
	EventHeader
	PromptID          *string `json:"prompt_id"`
	AgentID           *string `json:"agent_id"`
	AgentType         *string `json:"agent_type"`
	Phase             string  `json:"phase"` // start | stop
	Internal          bool    `json:"internal"`
	ClaimClass        *string `json:"claim_class"`
	StopHookActive    *bool   `json:"stop_hook_active"`
	BackgroundPresent *bool   `json:"background_present"`

	// The *_withheld keys, all oversize (1.2.11), as DeclarationV5's
	// (P-07 proposal, owner K).
	PromptIDWithheld  *string `json:"prompt_id_withheld"`
	AgentIDWithheld   *string `json:"agent_id_withheld"`
	AgentTypeWithheld *string `json:"agent_type_withheld"`
}

// Batch is written at PostToolBatch (1.2.10): the calls of one model step.
type Batch struct {
	EventHeader
	PromptID   *string  `json:"prompt_id"`
	ToolUseIDs []string `json:"tool_use_ids"`
	Size       int      `json:"size"`
	// PromptIDWithheld: oversize (P-07 proposal, owner K).
	PromptIDWithheld *string `json:"prompt_id_withheld"`
}

// Task is written at TaskCreated and TaskCompleted (1.2.10; archive 10.2.14).
type Task struct {
	EventHeader
	PromptID     *string `json:"prompt_id"`
	Event        string  `json:"event"` // created | completed
	TaskID       string  `json:"task_id"`
	Subject      *string `json:"subject"` // null when task_subjects is off
	AgentID      *string `json:"agent_id"`
	TeammateName *string `json:"teammate_name"`

	// The *_withheld keys, as DeclarationV5's (P-07 proposal, owner K).
	PromptIDWithheld *string `json:"prompt_id_withheld"` // oversize
	SubjectWithheld  *string `json:"subject_withheld"`   // switched_off (task_subjects) | masker_error
	AgentIDWithheld  *string `json:"agent_id_withheld"`  // oversize
}

// Compact is written at PostCompact (1.2.10): the summary's class, never
// its text.
type Compact struct {
	EventHeader
	PromptID          *string `json:"prompt_id"`
	Trigger           string  `json:"trigger"`
	SummaryClaimClass string  `json:"summary_claim_class"`
	SummaryBytes      int     `json:"summary_bytes"`
	PromptIDWithheld  *string `json:"prompt_id_withheld"` // oversize (P-07 proposal, owner K)
}

// Instructions is written at InstructionsLoaded (1.2.10; archive 10.2.14;
// LEARN reads file_sha256 over the first 1 MiB). FilePath is nullable so
// that a path over 1.2.11's 4,096 B cap can be withheld.
type Instructions struct {
	EventHeader
	PromptID        *string `json:"prompt_id"`
	AgentID         *string `json:"agent_id"`
	FilePath        *string `json:"file_path"`
	MemoryType      string  `json:"memory_type"`
	LoadReason      string  `json:"load_reason"`
	TriggerFilePath *string `json:"trigger_file_path"`
	GlobsCount      *int    `json:"globs_count"`
	FileSHA256      *string `json:"file_sha256"`

	// The *_withheld keys, as DeclarationV5's (P-07 proposal, owner K).
	PromptIDWithheld        *string `json:"prompt_id_withheld"`         // oversize
	AgentIDWithheld         *string `json:"agent_id_withheld"`          // oversize
	FilePathWithheld        *string `json:"file_path_withheld"`         // oversize | switched_off (paths)
	TriggerFilePathWithheld *string `json:"trigger_file_path_withheld"` // oversize | switched_off (paths)
}

// Model is written at PostModelSwitch (1.2.10).
type Model struct {
	EventHeader
	PromptID  *string `json:"prompt_id"`
	From      *string `json:"from"`
	To        string  `json:"to"`
	Requested *string `json:"requested"`
	Source    string  `json:"source"`
	// PromptIDWithheld: oversize (P-07 proposal, owner K).
	PromptIDWithheld *string `json:"prompt_id_withheld"`
}

// OutcomeJoin is BREADTH's append-only join of a call's outcome learned
// after its execution record (1.2.10 table; archive 60).
type OutcomeJoin struct {
	EventHeader
	ToolUseID     string `json:"tool_use_id"`
	Outcome       string `json:"outcome"`
	ExitCode      *int   `json:"exit_code"`
	OutcomeSource string `json:"outcome_source"`
	ParserVersion int    `json:"parser_version"`
}

// WriteObserved is BREADTH's record of a write seen outside a tool call
// (Cursor afterFileEdit; archive 60): the path, stored as every path is,
// and the edit count. Path is nullable so that it can be withheld as
// file_path is: over 1.2.11's 4,096 B cap, or with the paths switch off.
type WriteObserved struct {
	EventHeader
	Path         *string `json:"path"`
	EditCount    int     `json:"edit_count"`
	PathWithheld *string `json:"path_withheld"` // oversize | switched_off (paths)
}
