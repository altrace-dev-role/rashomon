package hook

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// PostPayload is the PostToolUse hook input, reduced to the fields an
// execution record is built from.
//
// There is deliberately no tool_response field. That field is tool output --
// what a file held, what a command printed -- and holding it as a value would
// put that content into memory this program controls, from where a panic
// value or a log line carries it to Claude Code's debug log. The payload is
// scanned once into spans (hookio.Intake) and no field here is filled from
// the response's span. The one thing read from it is read by
// backgroundedCall, for Bash only: whether two keys are present, as bits on
// their spans, so stdout, stderr and the task id are never a value here.
//
// tool_input IS held, as a subslice of the payload, for the executed digest:
// the post-rewrite input is the only evidence a rewriting hook leaves
// anywhere, and only its digest survives this function. internal/shape is
// still the only package that looks inside it. A canary test sweeps the
// store, both report renders and both hook streams for its contents.
type PostPayload struct {
	SessionID string
	ToolName  string
	ToolUseID string

	// HookEventName is what makes one subcommand serve two events (v2).
	// PostToolUse and PostToolUseFailure carry the same shape and are
	// dispatched here rather than by two separate installed command lines,
	// because two command lines would be two places for the attribution and
	// panic-barrier discipline to drift apart.
	HookEventName string

	// Error is the head of the failure message, at most errorHeadBytes, and
	// it is the ONE field on this path that comes close to content. It is
	// read and never stored: only a parsed integer exit code survives this
	// function.
	Error string

	// IsInterrupt distinguishes a user interrupt from a command that failed
	// on its own. Measured on Claude Code 2.1.258: present on the failure
	// event, false for an ordinary non-zero exit.
	IsInterrupt *bool

	// DurationMS is the client's own measurement of the call.
	DurationMS *int64

	// ToolInput is the input as it ACTUALLY RAN, after any hook rewrote it.
	ToolInput json.RawMessage

	// CWD is carried onto the coverage record so the project key is present
	// on every phase rather than only the two the probe writes.
	CWD string
}

// errorHeadBytes is how much of a failure message is unescaped. The exit
// code is on its first line and nothing here reads past the head, so a long
// message never becomes a long string (RES-6).
const errorHeadBytes = 8 << 10

// from fills pl from an intake. A field of the wrong type fails the whole
// payload before any field is used, as the struct decode did.
func (pl *PostPayload) from(in *hookio.Payload) error {
	if err := in.Strs([]*string{&pl.SessionID, &pl.ToolName, &pl.ToolUseID, &pl.HookEventName, &pl.CWD},
		hookio.SessionID, hookio.ToolName, hookio.ToolUseID, hookio.HookEventName, hookio.CWD); err != nil {
		return err
	}
	var err error
	if pl.IsInterrupt, err = in.Bool(hookio.IsInterrupt); err != nil {
		return err
	}
	if pl.DurationMS, err = in.Int(hookio.DurationMS); err != nil {
		return err
	}
	switch s := in.Spans[hookio.Error]; s.Kind {
	case jsonscan.Absent, jsonscan.Null:
	case jsonscan.String:
		head, _, herr := jsonscan.Head(in.Raw, s, errorHeadBytes, make([]byte, 0, errorHeadBytes))
		if herr != nil {
			return herr
		}
		pl.Error = string(head)
	default:
		return hookio.ErrType
	}
	pl.ToolInput = in.Value(hookio.ToolInput)
	return nil
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
// about its own panic. Like a Handler, it is for one invocation, used by one
// goroutine.
type Post struct {
	st  *store.Store
	now func() time.Time
	// lockWaits is Handler.lockWaits.
	lockWaits time.Duration

	sessionID string
	toolUseID string
	cwd       string
	// withheld is Handler.withheld for the execution record.
	withheld bool
}

// NewPost builds a Post. now is injectable for the same reason it is on
// Handler: a test pins the clock and compares two records byte for byte.
func NewPost(st *store.Store, now func() time.Time) *Post {
	return &Post{st: st, now: now, lockWaits: LockWaitTotal, sessionID: UnattributedSession}
}

// Capture reads one payload and writes the execution record.
//
// stdin is bounded exactly as the declaration path bounds it. Of the response
// only the presence of backgroundTaskId and backgroundedByUser is read, for a
// Bash call (backgroundResponse), but the whole of it does arrive: an
// unbounded read of a payload carrying a hundred megabytes of tool output is a
// runtime fatal error, which recover cannot catch and which exits 2.
func (p *Post) Capture(in io.Reader) error {
	// First, as in Handler.Capture.
	p.st.LimitLockWaits(p.lockWaits)
	fault.Inject(fault.PointPostStart)

	payload, err := hookio.Intake(in)
	if err != nil {
		return err
	}
	var pl PostPayload
	if err := pl.from(payload); err != nil {
		return err
	}

	// Attribute before anything else can fail, as the declaration path does.
	p.sessionID = attribute(pl.SessionID)
	p.withheld = len(pl.SessionID) > store.MaxIDBytes
	p.toolUseID = store.CapID(pl.ToolUseID)
	p.cwd = pl.CWD

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
		RulesVersion:  shape.RulesVersion,
	}
	// Derived through the same function the declaration path uses, with the
	// same per-install key, or the two digests would never be comparable. An
	// absent tool_input leaves it empty rather than digesting nothing, because
	// the digest of an empty input is a real value that would compare unequal
	// to every declaration and report every such call as rewritten.
	//
	// The same Derive gives the masking of the line as it ran, which a
	// rewriting hook may have changed from the declaration's.
	if len(pl.ToolInput) > 0 {
		s := shape.Derive(pl.ToolName, pl.ToolInput, p.st.Key())
		rec.ExecutedDigest = s.Digest
		rec.StatusMasked, rec.RunnerDigest = s.StatusMasked, s.RunnerDigest
	}
	if pl.HookEventName == FailureEvent {
		rec.Outcome = store.ExecFailed
		rec.IsInterrupt = pl.IsInterrupt
		rec.ExitCode = exitCode(pl.Error)
		if pl.IsInterrupt != nil && *pl.IsInterrupt {
			rec.Outcome = store.ExecInterrupted
		}
	}
	// A failure event means the command ended or its launch failed, so it was
	// never moved to the background, whatever its payload carries.
	if pl.ToolName == "Bash" && pl.HookEventName != FailureEvent {
		rec.Backgrounded = backgroundedCall(payload)
	}
	// The caps, as on the declaration path (REC-52).
	if store.CapExecution(&rec) {
		p.withheld = true
	}
	return p.st.AppendExecution(rec)
}

// backgroundedCall reports a Bash call whose PostToolUse fired while the
// command was still running in the background, so that its outcome is the
// launch's and not the command's. Measured on Claude Code 2.1.280, a command
// moved to the background when it reached its timeout and one launched with
// run_in_background both fire PostToolUse, and the response carries
// backgroundTaskId (the timeout case timedOutAfterMs as well); a foreground
// call carries neither. A Ctrl+B background is recognised only if Claude Code
// marks it with backgroundTaskId or backgroundedByUser, which was not
// measured; backgroundedByUser is read the same way as the task id.
//
// This is the whole of what is read from tool_response: two presence bits on
// spans, true when a key carries anything but null or false. The task id is
// never copied, and stdout and stderr are never touched. A response that is
// not an object is a foreground call: the flag is set only on evidence.
func backgroundedCall(in *hookio.Payload) bool {
	var bits [2]jsonscan.Span
	keys := jsonscan.NewKeySet("backgroundTaskId", "backgroundedByUser")
	if jsonscan.Field(in.Raw, in.Spans[hookio.ToolResponse], keys, bits[:]) != nil {
		return false
	}
	return bits[0].Set() || bits[1].Set()
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
//
// Only the first line is read. Measured on Claude Code 2.1.285, a command that
// printed anything fails with "Exit code 2\n<its output>", and reading the
// whole remainder as the number dropped the code from every such failure. The
// output after the newline is never looked at.
func exitCode(msg string) *int {
	if !strings.HasPrefix(msg, exitCodePrefix) {
		return nil
	}
	// Only the first line is read; whether a newline was found does not
	// matter, since a message with none is all first line.
	first, _, _ := strings.Cut(msg[len(exitCodePrefix):], "\n")
	digits := strings.TrimSpace(first)
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
	reason := closeReason(captureErr)
	if sig.Delivered() {
		reason = store.ReasonTerminatedBySignal
	}
	if reason == "" && p.withheld {
		reason = store.ReasonRecordOversize
	}

	// A post-phase coverage record that cannot be written is read as its
	// absence, as on the call phase (H-10).
	_ = p.st.AppendCoverage(BuildCoverage(p.st, p.sessionID, store.PhasePost, reason, p.cwd, p.now()))
}
