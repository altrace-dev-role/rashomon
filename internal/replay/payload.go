package replay

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// unsupported collects the reasons an event cannot be fed to the recorder.
type unsupported struct{ ev *Event }

func (u *unsupported) add(r Reason) { u.ev.Unsupported = append(u.ev.Unsupported, r) }

func (u *unsupported) when(cond bool, r Reason) {
	if cond {
		u.add(r)
	}
}

// version adds the reason a line's Claude Code version is not one this
// program has a fixture for (history.Supported), if it is not.
func (u *unsupported) version(v string) {
	switch {
	case v == "":
		u.add(NoVersion)
	case !history.Supported(v):
		u.add(UnsupportedVersion)
	}
}

// settle makes an Unsupported event a Gap: the first reason is the one
// reported. A skipped event is never silent.
func (e *Event) settle() {
	if len(e.Unsupported) > 0 {
		e.Gap = e.Unsupported[0]
	}
}

// marshal encodes a payload. HTML is not escaped, so a tool_input or an error
// text reaches the recorder as the bytes it was, up to the whitespace that
// compacting a raw value removes.
func marshal(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic("replay: a payload would not encode: " + err.Error()) // every field is a string, a bool or valid raw JSON
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// prePayload is the PreToolUse payload as hook.Payload reads it: the fields
// of hook.Payload.from, less agent_id and agent_type, which a main agent's
// call does not carry and a subagent's makes the event Unsupported.
type prePayload struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	CWD            string          `json:"cwd"`
	PromptID       string          `json:"prompt_id"`
	PermissionMode string          `json:"permission_mode"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
}

// postPayload is the PostToolUse and PostToolUseFailure payload as
// hook.PostPayload reads it, and transcript_path beside it, which PostPayload
// does not but the host guard's harness detection does (harness.Detect, from
// cmdPost's claudeHostGuard) when CLAUDE_CODE_ENTRYPOINT is not set. ToolResponse is set only for the two keys
// backgroundedCall reads, and Error and IsInterrupt only on a failure.
// duration_ms is not a field: see the package doc.
type postPayload struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	CWD            string          `json:"cwd"`
	PromptID       string          `json:"prompt_id"`
	PermissionMode string          `json:"permission_mode"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   *response       `json:"tool_response,omitempty"`
	ToolUseID      string          `json:"tool_use_id"`
	Error          *string         `json:"error,omitempty"`
	IsInterrupt    *bool           `json:"is_interrupt,omitempty"`
}

// response is the part of a Bash tool_response the recorder reads: whether
// either key is set. The value true stands for whatever the task id was; a
// presence bit does not depend on the value (hook.backgroundedCall).
type response struct {
	BackgroundTaskID   bool `json:"backgroundTaskId,omitempty"`
	BackgroundedByUser bool `json:"backgroundedByUser,omitempty"`
}

// recapPayload is the Stop and UserPromptSubmit payload as
// cmd/rashomon.readFinalMessage reads it, less the keys harness detection
// reads by presence (cursor_version, conversation_id, generation_id, turn_id,
// model), which a Claude Code payload does not carry.
type recapPayload struct {
	SessionID            string  `json:"session_id"`
	TranscriptPath       string  `json:"transcript_path"`
	CWD                  string  `json:"cwd"`
	HookEventName        string  `json:"hook_event_name"`
	LastAssistantMessage *string `json:"last_assistant_message,omitempty"`
}

// pre is the PreToolUse event for a call: always, whatever became of the
// call. A denied call, and one whose result was not read, have this event and
// no other.
func (w *walker) pre(c *history.Call) *Event {
	e := callEvent(PreToolUse, c.UsePos, c)
	e.Denied = c.Outcome == history.Denied
	e.Missing = c.Outcome == history.OutcomeNone
	u := &unsupported{ev: e}
	// A conflict between two results is the Post's to bear: the declaration
	// was made before either.
	u.version(c.Version)
	u.when(c.SessionID == "", NoSessionID)
	u.when(w.opt.TranscriptPath == "", NoTranscriptPath)
	u.when(c.Sidechain, SidechainAgentType)
	u.when(c.CWD == "", NoCWD)
	input := toolInput(c)
	u.when(input == nil, InputDropped)
	ctx := w.sc.at(c.UsePos.Line)
	promptID, mode := "", ""
	if ctx != nil {
		promptID, mode = ctx.promptID, ctx.mode
		u.when(ctx.permChange != 0 && ctx.permChange < c.UsePos.Line, PermissionModeChanged)
	}
	u.when(promptID == "", NoPromptID)
	u.when(mode == "", NoPermissionMode)
	if e.Supported() {
		e.Payload = marshal(&prePayload{SessionID: c.SessionID, TranscriptPath: w.opt.TranscriptPath, CWD: c.CWD,
			PromptID: promptID, PermissionMode: mode, HookEventName: PreToolUse.String(),
			ToolName: c.ToolName, ToolInput: input, ToolUseID: c.ToolUseID})
	}
	e.settle()
	if e.Gap == "" && c.Reason == history.Evicted {
		e.Gap = Evicted
	}
	return e
}

// post is the PostToolUse or PostToolUseFailure event for a call, or nil for
// one that has none: a denial is refused before it runs, and a call with no
// result read has no moment its execution was recorded at.
func (w *walker) post(c *history.Call) *Event {
	if c.Outcome == history.OutcomeNone || c.Outcome == history.Denied {
		return nil
	}
	failed := c.Outcome != history.OK
	kind := PostToolUse
	if failed {
		kind = PostToolUseFailure
	}
	e := callEvent(kind, c.ResultPos, c)
	u := &unsupported{ev: e}
	u.version(c.Version)
	u.when(c.Reason == history.ResultConflict, ResultConflict)
	u.when(c.SessionID == "", NoSessionID)
	u.when(w.opt.TranscriptPath == "", NoTranscriptPath)
	u.when(c.Sidechain, SidechainAgentType)
	u.when(c.ResultCWD == "", NoResultCWD)
	u.when(c.ResultPromptID == "", NoPromptID)
	input := toolInput(c)
	u.when(input == nil, InputDropped)
	mode := ""
	if ctx := w.sc.at(c.ResultPos.Line); ctx != nil {
		mode = ctx.mode
		u.when(ctx.permChange != 0 && ctx.permChange < c.ResultPos.Line, PermissionModeChanged)
	}
	u.when(mode == "", NoPermissionMode)

	p := &postPayload{SessionID: c.SessionID, TranscriptPath: w.opt.TranscriptPath, CWD: c.ResultCWD, PromptID: c.ResultPromptID, PermissionMode: mode,
		HookEventName: kind.String(), ToolName: c.ToolName, ToolInput: input, ToolUseID: c.ToolUseID}
	switch {
	case c.Outcome == history.Interrupted:
		u.add(InterruptedCall)
	case failed:
		// The error is the head of the result's text, which is all the
		// recorder reads of it (hook.errorHeadBytes). A user interrupt can
		// end a call as an error whose text says so; no fixture shows what
		// the payload of that is.
		msg := string(c.OutputHead)
		u.when(strings.HasPrefix(msg, interruptMarker), InterruptedCall)
		no := false
		p.Error, p.IsInterrupt = &msg, &no
	case c.ToolName == "Bash":
		// Only a Bash call's tool_response is read, and only for two presence
		// bits. A line the bits cannot be read off leaves them unknown, and a
		// launch the call asked to background whose result names neither is
		// one the live payload would have flagged (hook.backgroundedCall).
		u.when(!c.ResultObject, ResponseUnreadable)
		u.when(c.ResultObject && c.Backgrounded && !c.ResultBackgroundTaskID && !c.ResultBackgroundedByUser, BackgroundUnconfirmed)
		if c.ResultBackgroundTaskID || c.ResultBackgroundedByUser {
			p.ToolResponse = &response{BackgroundTaskID: c.ResultBackgroundTaskID, BackgroundedByUser: c.ResultBackgroundedByUser}
		}
	}
	if e.Supported() {
		e.Payload = marshal(p)
	}
	e.settle()
	return e
}

func callEvent(k Kind, at history.Pos, c *history.Call) *Event {
	return &Event{Kind: k, Pos: at, SessionID: c.SessionID, ToolUseID: c.ToolUseID, ToolName: c.ToolName,
		Seq: c.Seq, Grade: c.Grade, GradeReason: c.Reason}
}

// toolInput is the call's input as a valid raw value, nil when there is none
// to give: it was over history's bound, or the block had no input key.
func toolInput(c *history.Call) json.RawMessage {
	if len(c.ToolInput) == 0 {
		return nil
	}
	return c.ToolInput
}
