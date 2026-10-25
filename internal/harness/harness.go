// Package harness identifies which agent harness fired a hook invocation --
// Claude Code, Cursor, Codex, or something this product has never seen -- and
// what that harness lets rashomon see or do.
//
// This is the one detector (Technical spec 6.2.1, 6.2.2). Every other
// component that behaves differently per harness reads an Identity or a
// CapSet from here; none of them re-derives identity from a payload.
//
// The package depends on internal/hookio for the already-scanned intake
// payload and on the standard library only. It imports no networking
// package.
package harness

// ID is a closed set. Values are never derived from input: Detect returns
// one of the five below, never a string built from a payload field.
type ID string

const (
	// ClaudeCode is positive evidence: CLAUDE_CODE_ENTRYPOINT named a value
	// in the 2.1.280 allowlist.
	ClaudeCode ID = "claude_code"
	// ClaudeCodePresumed is no foreign marker plus at least one Claude Code
	// signal short of an allowlisted entrypoint: a transcript path under the
	// config directory's projects/ tree, a transcript path with Claude Code's
	// own projects/<dir>/<session-uuid>.jsonl shape wherever the config
	// directory is, or the CLAUDE_PROJECT_DIR the hook process is started
	// with (BRD-4 ruling, 2026-10-24: the detector gap is a bug).
	ClaudeCodePresumed ID = "claude_code_presumed"
	Cursor             ID = "cursor"
	Codex              ID = "codex"
	Unknown            ID = "unknown"
)

// Surface is where the harness is driven from. Closed set.
type Surface string

const (
	SurfaceCLI     Surface = "cli"     // CLAUDE_CODE_ENTRYPOINT=cli
	SurfaceIDE     Surface = "ide"     // claude-vscode
	SurfaceDesktop Surface = "desktop" // claude-desktop, local-agent
	SurfaceSDK     Surface = "sdk"     // sdk-ts, sdk-py, sdk-cli
	SurfaceCloud   Surface = "cloud"   // remote*, claude-code-github-action
	SurfaceUnknown Surface = "unknown"
)

// Evidence names WHICH key decided the identity. Key names only, never
// values: Detect never turns a marker's value into an Evidence, and the one
// value it does read for a different field (cursor_version, into Version)
// is never itself the evidence string.
type Evidence string

const (
	EvCursorVersion        Evidence = "cursor_version"
	EvConversationGenID    Evidence = "conversation_id+generation_id"
	EvTurnIDOnToolEvent    Evidence = "turn_id_on_tool_or_turn_event"
	EvModelOffSessionStart Evidence = "model_on_non_session_start_event"
	EvEntrypointEnv        Evidence = "claude_code_entrypoint_env"
	EvClaudeTranscript     Evidence = "transcript_under_claude_projects"
	// EvClaudeProjectDirEnv: CLAUDE_PROJECT_DIR is set and non-empty. Claude
	// Code starts every hook command with it (2.1.42 and 2.1.310 both build
	// the hook environment as the process environment plus this key).
	EvClaudeProjectDirEnv Evidence = "claude_project_dir_env"
	// EvClaudeTranscriptShape: transcript_path ends projects/<dir>/<uuid>.jsonl,
	// the layout Claude Code writes under whatever its config directory is.
	EvClaudeTranscriptShape Evidence = "transcript_projects_session_shape"
)

// Capability is one thing a harness lets rashomon see or do. Closed set
// (Technical spec 6.2.1). Values a capability table has not populated for a
// given (ID, Surface) read as Unverified by Caps, never Yes.
type Capability string

const (
	CapDeclarationBeforeRun Capability = "declaration_before_run"
	CapExitCode             Capability = "exit_code"
	CapFailureEvent         Capability = "failure_event"
	CapFinalMessage         Capability = "final_message"
	CapTurnKey              Capability = "turn_key"
	CapWritePaths           Capability = "write_paths"
	CapSubagentIdentity     Capability = "subagent_identity"
	CapMCPProvenance        Capability = "mcp_provenance"
	CapBackgroundState      Capability = "background_state"
	CapNudgePost            Capability = "nudge_post"
	CapNudgeFailure         Capability = "nudge_failure"
	CapNudgeSessionStart    Capability = "nudge_session_start"
	CapNudgeBatch           Capability = "nudge_batch"
	CapUserOnlyChannel      Capability = "user_only_channel"
	CapMirrorInBand         Capability = "mirror_in_band"
	CapDeny                 Capability = "deny"
)

// allCapabilities lists every Capability in a fixed order, so Blind and a
// CapSet built from scratch enumerate deterministically.
var allCapabilities = []Capability{
	CapDeclarationBeforeRun, CapExitCode, CapFailureEvent, CapFinalMessage,
	CapTurnKey, CapWritePaths, CapSubagentIdentity, CapMCPProvenance,
	CapBackgroundState, CapNudgePost, CapNudgeFailure, CapNudgeSessionStart,
	CapNudgeBatch, CapUserOnlyChannel, CapMirrorInBand, CapDeny,
}

// CapState is whether a capability is live.
type CapState string

const (
	Yes        CapState = "yes"
	No         CapState = "no"
	Unverified CapState = "unverified"
)

// CapSet is the capability table for one (ID, Surface) pair.
type CapSet map[Capability]CapState

// Identity is what one hook invocation decided about where it is running.
type Identity struct {
	ID          ID         `json:"id"`
	Surface     Surface    `json:"surface"`
	Evidence    []Evidence `json:"evidence"`     // sorted, de-duplicated
	Version     *string    `json:"version"`      // cursor_version only, charset-checked; else nil
	CapsVersion int        `json:"caps_version"` // version of the table Caps reads
	Blind       []string   `json:"blind"`        // capability names whose state is not "yes", sorted
}

// Env is the only environment Detect may read. Both fields are resolved by
// the caller: Detect never calls os.Getenv or settings.ConfigDir itself.
type Env struct {
	Entrypoint string // CLAUDE_CODE_ENTRYPOINT, or "" when unset
	ConfigDir  string // settings.ConfigDir()
	ProjectDir string // CLAUDE_PROJECT_DIR, or "" when unset
}
