// Package claudecode reads Claude Code's JSONL transcripts as trace steps. It
// is the one place Claude Code's transcript words are known: its record
// fields, its tool names, the openings of its tool errors, its denial kinds
// and the markers it writes in the person's slot.
package claudecode

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/trace"
)

// line is the part of a transcript record the reader decodes.
type line struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	Timestamp   string `json:"timestamp"`
	Cwd         string `json:"cwd"`
	// DenialKind is Claude Code's own word for a refused call (2.1.280):
	// "user-rejected" when the person said no at the prompt,
	// "permission-rule" when a hook or a settings rule refused it. A record
	// without it is read from the result's text (report.UserRejected).
	DenialKind string `json:"toolDenialKind"`
	// Origin marks who wrote a user record: kind "human" for words the
	// person typed; another kind for another session's message. Versions
	// before the field write none.
	Origin struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Message struct {
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
	Text      string          `json:"text"`
}

// Read sends one transcript's steps to fn in the order the file records them
// and returns how many records it stepped over for being longer than
// report.TranscriptMaxLine. A record that does not parse is passed over, and
// so are Claude Code's own context records (isMeta) and every record that is
// neither the person's nor the agent's. It fills no content field: a call's
// Command, Paths and Changes and a result's Output stay empty.
func Read(r io.Reader, fn func(trace.Step)) (skipped int, err error) {
	return read(r, fn, false)
}

// ReadContent is Read with the content fields filled: each call's command,
// paths and changes, and each result's output.
func ReadContent(r io.Reader, fn func(trace.Step)) (skipped int, err error) {
	return read(r, fn, true)
}

func read(r io.Reader, fn func(trace.Step), content bool) (skipped int, err error) {
	sc := linescan.New(r, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	for sc.Scan() {
		b := sc.Bytes()
		// Only user and assistant records carry steps; progress, snapshot
		// and summary records, most of a transcript's bytes, are passed
		// over before any decode.
		if !bytes.Contains(b, []byte(`"type":"user"`)) && !bytes.Contains(b, []byte(`"type":"assistant"`)) {
			continue
		}
		var l line
		if json.Unmarshal(b, &l) != nil {
			continue
		}
		base := trace.Step{Session: l.SessionID, At: stamp(l.Timestamp), Cwd: l.Cwd, Sidechain: l.IsSidechain}
		entry := base
		entry.Kind = trace.Entry
		fn(entry)
		switch l.Type {
		case "assistant":
			assistant(&l, base, fn, content)
		case "user":
			user(&l, base, fn, content)
		}
	}
	return sc.Skipped(), sc.Err()
}

func stamp(ts string) time.Time {
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}
	}
	return at
}

func assistant(l *line, base trace.Step, fn func(trace.Step), content bool) {
	var blocks []block
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	s := base
	s.Kind = trace.Message
	fn(s)
	for i := range blocks {
		b := &blocks[i]
		switch b.Type {
		case "tool_use":
			s := base
			s.Kind = trace.Call
			s.Call = &trace.CallStep{ID: b.ID, Tool: b.Name, Class: ActClass(b.Name), Input: b.Input}
			if content {
				fill(s.Call)
			}
			fn(s)
		case "text":
			s := base
			s.Kind, s.Text = trace.Text, b.Text
			fn(s)
		}
	}
}

func user(l *line, base trace.Step, fn func(trace.Step), content bool) {
	if l.IsMeta {
		return
	}
	c := bytes.TrimSpace(l.Message.Content)
	if len(c) > 0 && c[0] == '"' {
		var s string
		if json.Unmarshal(c, &s) == nil {
			fn(said(l, base, s))
		}
		return
	}
	var blocks []block
	if json.Unmarshal(c, &blocks) != nil {
		return
	}
	for i := range blocks {
		if blocks[i].Type == "tool_result" {
			fn(result(l, base, &blocks[i], content))
		}
	}
	for i := range blocks {
		if blocks[i].Type == "text" {
			fn(said(l, base, blocks[i].Text))
			return
		}
	}
}

// interruptMark is the line Claude Code writes when the person stops the
// agent mid-turn; a line that opens like it and says more ("... for tool
// use") follows the person refusing a call.
const interruptMark = "[Request interrupted by user]"

func said(l *line, base trace.Step, text string) trace.Step {
	s := base
	s.Kind, s.Text = trace.Said, text
	t := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(t, "<"):
		// Claude Code's markup in the person's slot: a slash command, its
		// output, a hook's message.
		s.Origin = trace.OriginMachine
	case l.Origin.Kind == "human":
		s.Origin = trace.OriginPerson
	case l.Origin.Kind != "":
		s.Origin = trace.OriginPeer
	}
	switch {
	case t == interruptMark:
		s.Signal = trace.SignalInterrupt
	case strings.HasPrefix(t, "[Request interrupted"):
		s.Signal = trace.SignalInterruptForTool
	}
	return s
}

func result(l *line, base trace.Step, b *block, content bool) trace.Step {
	s := base
	s.Kind = trace.Result
	r := &trace.ResultStep{CallID: b.ToolUseID, Failed: b.IsError}
	s.Result = r
	if content {
		r.Output, _, _ = report.ResultClass(b.IsError, b.Content)
	}
	if !b.IsError {
		return s
	}
	switch l.DenialKind {
	case "user-rejected":
		r.Denied = trace.DeniedByPerson
		return s
	case "permission-rule":
		r.Denied = trace.DeniedByRule
		return s
	case "":
		// A version before the field: the text says which, below.
	default:
		r.Denied = trace.DeniedUnrecognised
		return s
	}
	text, denied, _ := report.ResultClass(true, b.Content)
	r.Text = text
	switch {
	case denied && report.UserRejected(text):
		r.Denied = trace.DeniedByPerson
	case denied:
		r.Denied = trace.DeniedOther
	default:
		r.Error = ToolError(text)
	}
	return s
}

// ActClass is the class of a call by Claude Code's tool name.
func ActClass(name string) trace.ActClass {
	if strings.HasPrefix(name, "mcp__") {
		return trace.ActExternal
	}
	return toolClass[name]
}

// toolClass maps Claude Code's built-in tools onto act classes; a name not
// here is trace.ActOther.
var toolClass = map[string]trace.ActClass{
	"Bash":            trace.ActExec,
	"BashOutput":      trace.ActExecControl,
	"KillShell":       trace.ActExecControl,
	"Edit":            trace.ActEdit,
	"MultiEdit":       trace.ActEdit,
	"NotebookEdit":    trace.ActEdit,
	"Write":           trace.ActWrite,
	"Read":            trace.ActRead,
	"NotebookRead":    trace.ActRead,
	"Glob":            trace.ActSearch,
	"Grep":            trace.ActSearch,
	"WebFetch":        trace.ActFetch,
	"WebSearch":       trace.ActFetch,
	"Task":            trace.ActDelegate,
	"Agent":           trace.ActDelegate,
	"ExitPlanMode":    trace.ActPlan,
	"AskUserQuestion": trace.ActAsk,
}

// ToolError is the kind of tool error by the opening Claude Code writes for
// it, those measured on real transcripts (SEED1 approach note). A command's
// own failure (Exit code N) is not one.
func ToolError(text string) trace.ToolError {
	t := strings.TrimPrefix(text, "<tool_use_error>")
	if len(t) > 160 {
		t = t[:160]
	}
	switch {
	case strings.HasPrefix(t, "String to replace not found in file"):
		return trace.ErrorEditNotFound
	case strings.HasPrefix(t, "Found ") && strings.Contains(t, " matches of the string to replace"):
		return trace.ErrorEditAmbiguous
	case strings.HasPrefix(t, "No changes to make: old_string and new_string are exactly the same"):
		return trace.ErrorEditNoChange
	case strings.HasPrefix(t, "File has been modified since read"):
		return trace.ErrorStaleWrite
	case strings.HasPrefix(t, "File has not been read yet"):
		return trace.ErrorUnreadWrite
	case strings.HasPrefix(t, "File content (") && strings.Contains(t, "exceeds maximum"):
		return trace.ErrorTooLarge
	case strings.HasPrefix(t, "File does not exist"):
		return trace.ErrorNoSuchFile
	case strings.HasPrefix(t, "InputValidationError: "):
		return trace.ErrorBadInput
	}
	return trace.ErrorNone
}
