package acceptance

import (
	"encoding/json"
	"testing"
)

const (
	testSession   = "sess-1"
	testToolUseID = "toolu_1"
)

// payloadOpts builds a PreToolUse payload. Zero values mean "field absent",
// which is the case the pointer fields in the record exist to represent.
type payloadOpts struct {
	SessionID      string
	PromptID       string
	TranscriptPath string
	CWD            string
	ScratchpadDir  string
	PermissionMode string
	ToolName       string
	ToolInput      map[string]any
	ToolUseID      string
	AgentID        string
	AgentType      string
}

func defaultPayload() payloadOpts {
	return payloadOpts{
		SessionID:      testSession,
		PromptID:       "prompt-1",
		TranscriptPath: "/tmp/transcripts/sess-1.jsonl",
		CWD:            "/tmp/project",
		ScratchpadDir:  "/tmp/project/.scratch",
		PermissionMode: "default",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": "git status --short"},
		ToolUseID:      testToolUseID,
	}
}

func (o payloadOpts) build(t *testing.T) string {
	t.Helper()

	obj := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      o.SessionID,
		"transcript_path": o.TranscriptPath,
		"cwd":             o.CWD,
		"scratchpad_dir":  o.ScratchpadDir,
		"permission_mode": o.PermissionMode,
		"tool_name":       o.ToolName,
		"tool_input":      o.ToolInput,
		"tool_use_id":     o.ToolUseID,
	}
	// Omitted rather than empty: prompt_id is absent until the first input of a
	// session, and the agent fields appear only inside a subagent call.
	if o.PromptID != "" {
		obj["prompt_id"] = o.PromptID
	}
	if o.AgentID != "" {
		obj["agent_id"] = o.AgentID
	}
	if o.AgentType != "" {
		obj["agent_type"] = o.AgentType
	}

	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("building payload: %v", err)
	}
	return string(b)
}
