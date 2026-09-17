package hook

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/fault"
	"github.com/altrace-dev-role/altrace-attest/internal/store"
)

// PostPayload is the PostToolUse hook input, reduced to the three fields an
// execution record carries.
//
// There is deliberately no tool_response field. That field is tool output --
// what a file held, what a command printed -- and declaring it would unmarshal
// that content into memory this program controls, from where a panic value or
// a log line carries it to Claude Code's debug log. encoding/json discards a
// key no field claims, so the response is never a value here at all. The
// guarantee is structural: there is nothing to forget to redact.
//
// tool_input is absent for a smaller reason: post derives no shape, and the
// declaration this execution answers already carries the one derived from it.
type PostPayload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolUseID string `json:"tool_use_id"`
}

// Post captures one PostToolUse invocation and then closes it out.
//
// It is split in two for the same reason Handler is: Close runs on paths where
// Capture did not return normally, and a single function cannot promise that
// about its own panic.
type Post struct {
	st  *store.Store
	now func() time.Time

	sessionID string
}

// NewPost builds a Post. now is injectable for the same reason it is on
// Handler: a test pins the clock and compares two records byte for byte.
func NewPost(st *store.Store, now func() time.Time) *Post {
	return &Post{st: st, now: now, sessionID: UnattributedSession}
}

// Capture reads one payload and writes the execution record.
//
// stdin is bounded exactly as the declaration path bounds it. The response is
// not read, but it does arrive: an unbounded read of a payload carrying a
// hundred megabytes of tool output is a runtime fatal error, which recover
// cannot catch and which exits 2.
func (p *Post) Capture(in io.Reader) error {
	fault.Inject(fault.PointPostStart)

	raw, err := readPayload(in)
	if err != nil {
		return err
	}

	var pl PostPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return err
	}

	// Attribute before anything else can fail, as the declaration path does.
	if pl.SessionID != "" {
		p.sessionID = pl.SessionID
	}

	fault.Inject(fault.PointPostParsed)

	return p.st.AppendExecution(store.Execution{
		Type:          store.TypeExecution,
		SchemaVersion: store.SchemaVersion,
		RecordedAtMS:  p.now().UnixMilli(),
		ToolUseID:     pl.ToolUseID,
		SessionID:     p.sessionID,
		ToolName:      pl.ToolName,
	})
}

// Close writes the coverage record for this invocation. There is no terminal
// record to write: an execution record is not an entry that something later
// closes, it is the close.
func (p *Post) Close(sig *Signals, captureErr error) {
	reason := ""
	switch {
	case sig.Delivered():
		reason = store.ReasonTerminatedBySignal
	case errors.Is(captureErr, store.ErrLockTimeout):
		reason = store.ReasonLockTimeout
	case captureErr != nil:
		reason = store.ReasonInternalError
	}

	_ = p.st.AppendCoverage(BuildCoverage(p.st, p.sessionID, store.PhasePost, reason, p.now()))
}
