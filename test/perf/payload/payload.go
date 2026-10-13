// Package payload generates the hook payloads the perf gates and benchmarks
// run against, in code and at exact byte sizes, so a case named "8 MiB" is
// 8,388,608 bytes and not a fixture file that drifted (archive 85.1.2: a
// payload 263 bytes over the cap once measured the refusal path as if it
// were an accepted 8 MiB).
package payload

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Session is the session id every generated payload carries.
const Session = "4b1f0c2e-9a7d-4e3b-8c21-5d6f7a8b9c0d"

const (
	transcript = "/Users/dev/.claude/projects/-Users-dev-src-project/" + Session + ".jsonl"
	cwd        = "/Users/dev/src/project"
)

// envelope is the common head of every hook payload.
type envelope struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	HookEventName  string `json:"hook_event_name"`
}

func head(event string) envelope {
	return envelope{SessionID: Session, TranscriptPath: transcript, CWD: cwd, PermissionMode: "default", HookEventName: event}
}

type toolPayload struct {
	envelope
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolUseID    string          `json:"tool_use_id"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
}

// BashPre is a short PreToolUse Bash payload.
func BashPre(command, toolUseID string) []byte {
	in, _ := json.Marshal(map[string]string{"command": command, "description": "Show working tree status"})
	return must(toolPayload{envelope: head("PreToolUse"), ToolName: "Bash", ToolInput: in, ToolUseID: toolUseID})
}

// BashPost is the PostToolUse payload for BashPre's call.
func BashPost(command, toolUseID, stdout string) []byte {
	in, _ := json.Marshal(map[string]string{"command": command, "description": "Show working tree status"})
	resp, _ := json.Marshal(map[string]any{"stdout": stdout, "stderr": "", "interrupted": false, "isImage": false})
	return must(toolPayload{envelope: head("PostToolUse"), ToolName: "Bash", ToolInput: in, ToolUseID: toolUseID, ToolResponse: resp})
}

// BashPreExact is a PreToolUse Bash payload padded in its command so the
// whole payload is exactly total bytes: the shape of the 400 B class, which
// an Edit payload's fixed part (over 470 bytes) cannot reach.
func BashPreExact(total int, toolUseID string) ([]byte, error) {
	return exact(total, func(pad string) []byte { return BashPre("git status --short "+pad, toolUseID) })
}

// BashPostExact is the PostToolUse Bash payload padded in its stdout.
func BashPostExact(total int, toolUseID string) ([]byte, error) {
	return exact(total, func(pad string) []byte { return BashPost("git status --short", toolUseID, pad) })
}

// smallShape is the size up to which a case uses the Bash shape; above it the
// payload is an Edit, which is what carries large tool input and output.
const smallShape = 4 << 10

// Pre is a PreToolUse payload of exactly total bytes in the shape its size
// calls for; Post is the same for PostToolUse.
func Pre(total int, toolUseID string) ([]byte, error) {
	if total <= smallShape {
		return BashPreExact(total, toolUseID)
	}
	return EditPre(total, toolUseID)
}

func Post(total int, toolUseID string) ([]byte, error) {
	if total <= smallShape {
		return BashPostExact(total, toolUseID)
	}
	return EditPost(total, toolUseID)
}

// EditPre is a PreToolUse Edit payload whose new_string is padded so the
// whole payload is exactly total bytes. total must leave room for the fixed
// part; the error says how much that is.
func EditPre(total int, toolUseID string) ([]byte, error) {
	build := func(pad string) []byte {
		in, _ := json.Marshal(map[string]any{
			"file_path":   cwd + "/internal/batch/handle.go",
			"old_string":  "// handle1 processes one batch of records\n",
			"new_string":  pad,
			"replace_all": false,
		})
		return must(toolPayload{envelope: head("PreToolUse"), ToolName: "Edit", ToolInput: in, ToolUseID: toolUseID})
	}
	return exact(total, build)
}

// EditPost is a PostToolUse Edit payload whose tool_response carries the
// file's new content, padded so the whole payload is exactly total bytes.
func EditPost(total int, toolUseID string) ([]byte, error) {
	build := func(pad string) []byte {
		in, _ := json.Marshal(map[string]any{
			"file_path":   cwd + "/internal/batch/handle.go",
			"old_string":  "// handle1 processes one batch of records\n",
			"new_string":  "// handle1 processes one batch of records, in order\n",
			"replace_all": false,
		})
		resp, _ := json.Marshal(map[string]any{
			"filePath":     cwd + "/internal/batch/handle.go",
			"oldString":    "// handle1 processes one batch of records\n",
			"newString":    "// handle1 processes one batch of records, in order\n",
			"originalFile": pad,
			"userModified": false,
			"replaceAll":   false,
		})
		return must(toolPayload{envelope: head("PostToolUse"), ToolName: "Edit", ToolInput: in, ToolUseID: toolUseID, ToolResponse: resp})
	}
	return exact(total, build)
}

// Stop is the Stop payload for a turn that ended with lastAssistantMessage.
func Stop(lastAssistantMessage string) []byte {
	return must(struct {
		envelope
		StopHookActive       bool   `json:"stop_hook_active"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}{head("Stop"), false, lastAssistantMessage})
}

// Prompt is the UserPromptSubmit payload for prompt.
func Prompt(prompt string) []byte {
	return must(struct {
		envelope
		Prompt string `json:"prompt"`
	}{head("UserPromptSubmit"), prompt})
}

// SessionStart and SessionEnd are the probe payloads.
func SessionStart() []byte {
	return must(struct {
		envelope
		Source string `json:"source"`
	}{head("SessionStart"), "startup"})
}

func SessionEnd() []byte {
	return must(struct {
		envelope
		Reason string `json:"reason"`
	}{head("SessionEnd"), "other"})
}

// exact pads build's variable field until the payload is exactly total
// bytes. The pad is deterministic text: 63-byte lines of Go-looking source,
// so the JSON escaping is predictable (no quotes or backslashes), and the
// size converges in one step after measuring the fixed part.
func exact(total int, build func(pad string) []byte) ([]byte, error) {
	fixed := len(build(""))
	if total < fixed {
		return nil, fmt.Errorf("payload: %d bytes is below the fixed part, %d", total, fixed)
	}
	pad := filler(total - fixed)
	out := build(pad)
	if len(out) != total {
		return nil, fmt.Errorf("payload: built %d bytes, want %d", len(out), total)
	}
	return out, nil
}

// filler returns n bytes of text that JSON encodes to exactly n bytes: the
// line pattern has no byte JSON escapes (no quote, backslash, tab, newline,
// '<', '>' or '&'), so lines are joined with a space instead of a newline.
func filler(n int) string {
	const line = "    for i := range batch { if err := handle(batch[i]); err != nil { return err } } "
	var sb strings.Builder
	sb.Grow(n)
	for sb.Len()+len(line) <= n {
		sb.WriteString(line)
	}
	for sb.Len() < n {
		sb.WriteByte('x')
	}
	return sb.String()
}

func must(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // a payload struct that cannot marshal is a defect in this package, not a runtime condition
	}
	return b
}
