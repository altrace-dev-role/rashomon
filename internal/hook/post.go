package hook

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
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
// tool_input IS declared, as of the executed-digest field below. It was absent
// while post derived no shape; it is present now because the post-rewrite
// input is the only evidence a rewriting hook leaves anywhere, and only its
// digest survives this function.
type PostPayload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolUseID string `json:"tool_use_id"`

	// HookEventName is what makes one subcommand serve two events (v2).
	// PostToolUse and PostToolUseFailure carry the same shape and are
	// dispatched here rather than by two separate installed command lines,
	// because two command lines would be two places for the attribution and
	// panic-barrier discipline to drift apart.
	HookEventName string `json:"hook_event_name"`

	// Error is the failure message, and it is the ONE field on this path that
	// comes close to content. It is read and never stored: only a parsed
	// integer exit code survives this function. The message itself can carry a
	// fragment of what the command printed, which is exactly why there is no
	// record field it could be assigned to.
	Error string `json:"error"`

	// IsInterrupt distinguishes a user interrupt from a command that failed on
	// its own. Measured on Claude Code 2.1.258: present on the failure event,
	// false for an ordinary non-zero exit.
	IsInterrupt *bool `json:"is_interrupt"`

	// DurationMS is the client's own measurement of the call.
	DurationMS *int64 `json:"duration_ms"`

	// ToolInput is the input as it ACTUALLY RAN, after any hook rewrote it.
	//
	// Declaring it is a deliberate reversal of the original note above, and the
	// asymmetry with tool_response is the whole reason it is safe. tool_response
	// is tool OUTPUT and nothing here needs it, so the guarantee for it stays
	// structural: no field claims it, so it is never a value in this process.
	// tool_input is different -- it is the one field that can show a rewriting
	// hook, and the declaration path already unmarshals and digests the same
	// field. Only the digest survives this function; internal/shape is still
	// the only package that looks inside it. A canary test sweeps the store,
	// both report renders and both hook streams for this field's contents.
	ToolInput json.RawMessage `json:"tool_input"`
}

// FailureEvent is the hook event name Claude Code fires instead of
// PostToolUse when a tool call ends badly.
//
// Measured on 2.1.258: a failing Bash call fires this event and NOT
// PostToolUse, carrying error: "Exit code 1", is_interrupt: false and
// duration_ms, with no tool_response. Subscribing to PostToolUse alone is
// therefore not a partial view of failures, it is a complete absence of them:
// every failed call would have a declaration and no execution, which is the
// same shape as a call the user denied.
const FailureEvent = "PostToolUseFailure"

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

	rec := store.Execution{
		Type:          store.TypeExecution,
		SchemaVersion: store.SchemaVersion,
		RecordedAtMS:  p.now().UnixMilli(),
		ToolUseID:     pl.ToolUseID,
		SessionID:     p.sessionID,
		ToolName:      pl.ToolName,
		Outcome:       store.ExecOK,
		DurationMS:    positive(pl.DurationMS),
	}
	// Derived through the same function the declaration path uses, with the
	// same per-install key, or the two digests would never be comparable. An
	// absent tool_input leaves it empty rather than digesting nothing, because
	// the digest of an empty input is a real value that would compare unequal
	// to every declaration and report every such call as rewritten.
	if len(pl.ToolInput) > 0 {
		rec.ExecutedDigest = shape.Derive(pl.ToolName, pl.ToolInput, p.st.Key()).Digest
	}
	if pl.HookEventName == FailureEvent {
		rec.Outcome = store.ExecFailed
		rec.IsInterrupt = pl.IsInterrupt
		rec.ExitCode = exitCode(pl.Error)
		if pl.IsInterrupt != nil && *pl.IsInterrupt {
			rec.Outcome = store.ExecInterrupted
		}
	}
	return p.st.AppendExecution(rec)
}

// exitCodePrefix is the whole of what is parsed out of a failure message.
//
// The message is the only field on this path that can carry a fragment of what
// a command printed, so what leaves this function is an integer or nothing.
// Matching a fixed prefix rather than searching the message means a command
// whose OUTPUT happens to contain the words "Exit code 137" cannot put a
// number into the record.
const exitCodePrefix = "Exit code "

// exitCode parses the process exit status out of a failure message.
//
// Returns nil rather than 0 for every shape it does not recognise. Zero is an
// exit status that means success, so writing it for "no code was stated" would
// record the opposite of what happened -- and this record's whole purpose is to
// say that the call did not succeed.
func exitCode(msg string) *int {
	if !strings.HasPrefix(msg, exitCodePrefix) {
		return nil
	}
	digits := strings.TrimSpace(msg[len(exitCodePrefix):])
	if digits == "" {
		return nil
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n == 0 {
		return nil
	}
	return &n
}

// positive returns d only when it is a duration we actually have. A zero or
// negative duration is not a measurement, and recording 0 would claim a call
// took no time rather than that nobody timed it.
func positive(d *int64) *int64 {
	if d == nil || *d <= 0 {
		return nil
	}
	return d
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
