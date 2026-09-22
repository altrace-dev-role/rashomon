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

// postOpts builds a PostToolUse payload. It carries the same session fields as
// the PreToolUse one plus tool_response, which is tool output: every test that
// uses this builder is checking what happens to that field, so it is always
// present and never empty.
type postOpts struct {
	SessionID      string
	TranscriptPath string
	PermissionMode string
	ToolName       string
	ToolInput      map[string]any
	ToolUseID      string
	ToolResponse   any
}

func defaultPost() postOpts {
	return postOpts{
		SessionID:      testSession,
		TranscriptPath: "/tmp/transcripts/sess-1.jsonl",
		PermissionMode: "default",
		ToolName:       "Bash",
		ToolInput:      map[string]any{"command": "git status --short"},
		ToolUseID:      testToolUseID,
		ToolResponse: map[string]any{
			"stdout":      " M README.md\n",
			"stderr":      "",
			"interrupted": false,
		},
	}
}

func (o postOpts) build(t *testing.T) string {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      o.SessionID,
		"transcript_path": o.TranscriptPath,
		"cwd":             "/tmp/project",
		"permission_mode": o.PermissionMode,
		"tool_name":       o.ToolName,
		"tool_input":      o.ToolInput,
		"tool_use_id":     o.ToolUseID,
		"tool_response":   o.ToolResponse,
	})
	if err != nil {
		t.Fatalf("building payload: %v", err)
	}
	return string(b)
}

// stopPayload builds a Stop/StopFailure hook body. last_assistant_message is
// the field Part 3's digest and Part 4's recap both read straight off stdin
// -- never a transcript path this reader opens itself (see internal/digest's
// package doc). stopHookActive is carried for completeness; recap does not
// gate on it (the spec is explicit that it guards recursion, not duplicate
// output -- see H-101's idempotency key instead).
func stopPayload(sessionID, lastAssistantMessage string, stopHookActive bool) string {
	b, err := json.Marshal(map[string]any{
		"hook_event_name":        "Stop",
		"session_id":             sessionID,
		"transcript_path":        "/tmp/transcripts/" + sessionID + ".jsonl",
		"stop_hook_active":       stopHookActive,
		"last_assistant_message": lastAssistantMessage,
	})
	if err != nil {
		panic(err)
	}
	return string(b)
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
