package backtest

import (
	"strings"
	"testing"
	"time"
)

const transcript = `{"type":"user","sessionId":"s1","promptId":"p1","permissionMode":"acceptEdits","timestamp":"2026-10-29T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","sessionId":"s1","cwd":"/w","timestamp":"2026-10-29T10:00:01Z","message":{"content":[{"type":"tool_use","id":"g","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","sessionId":"s1","timestamp":"2026-10-29T10:00:03Z","toolUseResult":{"stdout":"ok"},"message":{"content":[{"type":"tool_result","tool_use_id":"g","content":"ok","is_error":false}]}}
{"type":"assistant","isSidechain":true,"sessionId":"s1","timestamp":"2026-10-29T10:00:04Z","message":{"content":[{"type":"tool_use","id":"side","name":"Bash","input":{}}]}}
{"type":"assistant","sessionId":"s1","cwd":"/w","timestamp":"2026-10-29T10:00:05Z","message":{"content":[{"type":"tool_use","id":"d","name":"Bash","input":{}},{"type":"tool_use","id":"u","name":"Edit","input":{}},{"type":"tool_use","id":"x","name":"Read","input":{}}]}}
not a line
{"type":"user","sessionId":"s1","timestamp":"2026-10-29T10:00:09Z","message":{"content":[{"type":"tool_result","tool_use_id":"d","content":"The user doesn't want to proceed with this tool use. The tool use was rejected","is_error":true},{"type":"tool_result","tool_use_id":"u","content":"<tool_use_error>File has not been read yet</tool_use_error>","is_error":true}]}}
`

// TestP6_CallsReadATranscript: the calls a transcript holds, in order, as the
// hooks saw them: a run result is done, a denial ran only its PreToolUse, a
// refusal before any hook ran nothing, a call with no result is not done,
// and a subagent's sidechain call is not the session's. Each takes the prompt
// id and permission mode last carried before it.
func TestP6_CallsReadATranscript(t *testing.T) {
	session, calls, err := Calls(strings.NewReader(transcript))
	if err != nil || session != "s1" {
		t.Fatalf("Calls = %q, %v", session, err)
	}
	var got []string
	for _, c := range calls {
		got = append(got, c.ToolUseID)
	}
	if strings.Join(got, " ") != "g d u x" {
		t.Fatalf("calls %v, want g d u x", got)
	}
	g, d, u, x := calls[0], calls[1], calls[2], calls[3]
	if !g.Done || g.IsError || string(g.Response) != `{"stdout":"ok"}` || g.Cwd != "/w" || g.PromptID != "p1" ||
		g.Permission != "acceptEdits" || g.DoneAt.Sub(g.At) != 2*time.Second {
		t.Errorf("g = %+v", g)
	}
	if d.Done || !d.Denied || u.Done || !u.Unhooked || x.Done || x.Denied || x.Unhooked {
		t.Errorf("d = %+v, u = %+v, x = %+v", d, u, x)
	}
}
