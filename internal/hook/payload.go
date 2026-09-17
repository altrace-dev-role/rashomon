// Package hook turns a PreToolUse payload into records.
package hook

import "encoding/json"

// Payload is the PreToolUse hook input.
//
// All twelve documented fields are declared, including the ones that are not
// persisted, because a field that is read and discarded is easier to audit than
// a field nobody knew existed. ToolInput stays raw: only shape.Derive is
// allowed to look inside it.
type Payload struct {
	SessionID      string          `json:"session_id"`
	PromptID       string          `json:"prompt_id"`
	TranscriptPath string          `json:"transcript_path"`
	CWD            string          `json:"cwd"`
	ScratchpadDir  string          `json:"scratchpad_dir"`
	PermissionMode string          `json:"permission_mode"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
	AgentID        string          `json:"agent_id"`
	AgentType      string          `json:"agent_type"`
}
