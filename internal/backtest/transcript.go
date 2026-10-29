// Package backtest replays a session's transcript copy (the body store, card
// 4.1) through this build's hooks against a scratch store and reports the
// fires of one sensor, a steer pattern, beside the live steer log's for the
// same session (card P6, 6.3; PRD PF-RC14). It reads the user's store and
// writes only the scratch store, removed when the replay ends; it has no
// transport (owner rule 10-23).
package backtest

import (
	"cmp"
	"encoding/json"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
)

// Call is one tool call as the transcript holds it: the tool_use block, and
// the tool_result block that answered it, if one did.
type Call struct {
	ToolUseID, Name           string
	Input                     json.RawMessage
	Cwd, PromptID, Permission string
	At                        time.Time
	// Done is a result that ran: not a denial, not a refusal before any hook.
	Done, Denied, Unhooked, IsError bool
	Error                           string
	Response                        json.RawMessage
	DoneAt                          time.Time
}

// line is the part of a transcript line the replay reads.
type line struct {
	Type          string          `json:"type"`
	SessionID     string          `json:"sessionId"`
	Cwd           string          `json:"cwd"`
	Timestamp     time.Time       `json:"timestamp"`
	PromptID      string          `json:"promptId"`
	Permission    string          `json:"permissionMode"`
	IsSidechain   bool            `json:"isSidechain"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	Message       struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

// Calls reads a main-session transcript: its session id and its tool calls
// in the order the model asked for them. A sidechain line is a subagent's and
// is not replayed; a line that does not parse, or is longer than
// report.TranscriptMaxLine, is skipped, as the report's reader skips one.
// The prompt id and permission mode are the latest a line carried before the
// call.
func Calls(r io.Reader) (session string, calls []Call, err error) {
	sc := linescan.New(r, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	index := map[string]int{}
	var prompt, mode string
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.IsSidechain {
			continue
		}
		if session == "" {
			session = l.SessionID
		}
		prompt, mode = cmp.Or(l.PromptID, prompt), cmp.Or(l.Permission, mode)
		var blocks []block
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch {
			case l.Type == "assistant" && b.Type == "tool_use" && b.ID != "":
				index[b.ID] = len(calls)
				calls = append(calls, Call{ToolUseID: b.ID, Name: b.Name, Input: b.Input, Cwd: l.Cwd,
					PromptID: prompt, Permission: cmp.Or(mode, "default"), At: l.Timestamp})
			case l.Type == "user" && b.Type == "tool_result":
				i, ok := index[b.ToolUseID]
				if !ok {
					continue
				}
				c := &calls[i]
				c.Error, c.Denied, c.Unhooked = report.ResultClass(b.IsError, b.Content)
				c.Done, c.IsError, c.Response, c.DoneAt = !c.Denied && !c.Unhooked, b.IsError, l.ToolUseResult, l.Timestamp
			}
		}
	}
	return session, calls, sc.Err()
}
