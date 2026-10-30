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
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// PostPayload is the PostToolUse hook input, reduced to the fields an
// execution record is built from.
//
// There is deliberately no tool_response field. That field is tool output --
// what a file held, what a command printed -- and holding it as a value would
// put that content into memory this program controls, from where a panic
// value or a log line carries it to Claude Code's debug log. The payload is
// walked once into spans (hookio.Intake) and no field here is filled from
// the response's span. Two things are read from it, and neither is held:
// backgroundedCall reads, for Bash only, whether two keys are present, as
// bits on their spans, so the task id is never a value here; and gateOutput
// reads the output of a gate run, the call shape.GateRun names -- stdout,
// then stderr where stdout gives no summary, or a failure event's error after
// its first line -- decoded into a buffer local to that call, at most
// gateout.MaxScanBytes of it, cleared before it returns. What survives is
// gate_output's three closed words; no other call's output is read at all.
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

	// PromptID, AgentID and PermissionMode are read for the steer step only
	// (card 2.3 R5): the prompt a note is budgeted against, the subagent the
	// call ran in, and whether anyone is there to approve. The execution
	// record is unchanged; its declaration carries all three.
	PromptID       string
	AgentID        string
	PermissionMode string
}

// errorHeadBytes is how much of a failure message is unescaped. The exit
// code is on its first line and nothing here reads past the head, so a long
// message never becomes a long string (RES-6).
const errorHeadBytes = 8 << 10

// from fills pl from an intake. A field of the wrong type fails the whole
// payload before any field is used, as the struct decode did.
func (pl *PostPayload) from(in *hookio.Payload) error {
	if err := in.Strs(hookio.Into(hookio.SessionID, &pl.SessionID), hookio.Into(hookio.ToolName, &pl.ToolName),
		hookio.Into(hookio.ToolUseID, &pl.ToolUseID), hookio.Into(hookio.HookEventName, &pl.HookEventName),
		hookio.Into(hookio.CWD, &pl.CWD), hookio.Into(hookio.PromptID, &pl.PromptID), hookio.Into(hookio.AgentID, &pl.AgentID),
		hookio.Into(hookio.PermissionMode, &pl.PermissionMode)); err != nil {
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
	// capExec is Handler.capDecl for the execution record.
	capExec func(*store.Execution) bool

	sessionID string
	toolUseID string
	cwd       string
	// sp is the spec the steer step discovered for this call (DiscoverSpec),
	// nil for none or before the step runs (card 3.1 (c)).
	sp *spec.Compiled
	// withheld is Handler.withheld for the execution record.
	withheld bool

	// What the steer step reads (card 2.3 R5): the payload's prompt, agent,
	// permission mode and event, the seq the execution landed at in the
	// ordered stream (0 when it spilled or did not land), whether it
	// carried a runner digest, whether its tool input names
	// .claude/rashomon (storeNamed; card P5B, 5.7), and a success event's
	// call as the stale trigger reads its writes (writtenCall; card
	// STALE-NUDGE1).
	promptID       string
	agentID        string
	permissionMode string
	event          string
	landedSeq      int64
	landedRunner   bool
	landedStore    bool
	landedWrite    writtenCall
}

// NewPost builds a Post. now is injectable for the same reason it is on
// Handler: a test pins the clock and compares two records byte for byte.
func NewPost(st *store.Store, now func() time.Time) *Post {
	return &Post{st: st, now: now, lockWaits: LockWaitTotal, capExec: store.CapExecution,
		sessionID: store.UnattributedSession}
}

// Capture reads one payload and writes the execution record -- or, for a
// payload that names no session, none at all; see the guard below.
//
// stdin is bounded exactly as the declaration path bounds it. Of the response
// only the presence of backgroundTaskId and backgroundedByUser is read, for a
// Bash call (backgroundedCall), and a gate run's output, bounded and dropped
// (gateOutput), but the whole of it does arrive: an
// unbounded read of a payload carrying a hundred megabytes of tool output is a
// runtime fatal error, which recover cannot catch and which exits 2.
func (p *Post) Capture(in io.Reader) error {
	p.prologue()
	payload, err := hookio.Intake(in)
	if err != nil {
		if s, cwd, ok := refusedSession(err); ok {
			p.sessionID, p.cwd = s, cwd
		}
		return err
	}
	return p.capture(payload)
}

// CapturePayload is Capture for a caller that already has a parsed
// *hookio.Payload in hand -- see Handler.CapturePayload's doc, which this
// mirrors exactly.
func (p *Post) CapturePayload(payload *hookio.Payload) error {
	p.prologue()
	return p.capture(payload)
}

// prologue is Capture and CapturePayload's shared setup; see Handler.
// prologue's doc, which this mirrors exactly.
func (p *Post) prologue() {
	p.st.LimitLockWaits(p.lockWaits)
	fault.Inject(fault.PointPostStart)
}

// capture is Capture and CapturePayload's shared body.
func (p *Post) capture(payload *hookio.Payload) error {
	var pl PostPayload
	if err := pl.from(payload); err != nil {
		return err
	}

	// Attribute before anything else can fail, as the declaration path does.
	p.sessionID = attribute(pl.SessionID)
	p.withheld = len(pl.SessionID) > store.MaxIDBytes
	p.toolUseID = store.CapID(pl.ToolUseID)
	p.cwd = pl.CWD
	p.promptID, p.agentID, p.permissionMode, p.event = pl.PromptID, pl.AgentID, pl.PermissionMode, pl.HookEventName

	fault.Inject(fault.PointPostParsed)

	// A payload that names no session writes NO execution record: not ok, not
	// anything else in its place.
	//
	// Outcome is decided by which event fired -- PostToolUse is ok,
	// PostToolUseFailure is failed -- and that pairing was measured on
	// Claude Code, which sends session_id with every hook. A payload without
	// one came from something else, and nothing says the pairing holds there.
	// Cursor is the documented case -- per its documentation; nobody has run
	// Cursor against this recorder. It runs these hooks from
	// ~/.claude/settings.json, names a conversation_id instead, and
	// documents its postToolUse as "called after successful tool execution",
	// beside a postToolUseFailure of its own that its Claude-format mapping
	// omits. So a Cursor command that fails reaches this path as PostToolUse
	// or not at all, and nobody has measured which. The guard is right
	// either way: if a failure arrives here, ok is false; if only successes
	// do, ok is true by a rule read in another product's documentation, not
	// measured, and an outcome is the one thing this path records only on
	// measurement. Recording ok -- which this path did -- is the default
	// Execution.Outcome's own comment forbids: a call whose ending nobody
	// observed, claimed to have worked.
	//
	// Nothing is written in its place, and an outcome of "failed" would be no
	// better: the event pairing is the only evidence this path has, and it is
	// the part that does not hold for this payload. What remains is the
	// declaration the PreToolUse half left in the unattributed bucket -- the
	// report counts those as calls that arrived without a session id -- and
	// the coverage record Close writes for this invocation, so the post still
	// leaves a trace that it ran.
	if pl.SessionID == "" {
		return nil
	}

	// start is also when a gate run's output read begins its budget.
	start := p.now()
	rec := store.Execution{
		Type:          store.TypeExecution,
		SchemaVersion: store.SchemaVersion,
		RecordedAtMS:  start.UnixMilli(),
		ToolUseID:     pl.ToolUseID,
		SessionID:     p.sessionID,
		ToolName:      pl.ToolName,
		Outcome:       store.ExecOK,
		DurationMS:    positive(pl.DurationMS),
		RulesVersion:  shape.RulesVersion,
	}
	rec.Writer = Writer
	// Derived through the same function the declaration path uses, with the
	// same per-install key, or the two digests would never be comparable. An
	// absent tool_input leaves it empty rather than digesting nothing, because
	// the digest of an empty input is a real value that would compare unequal
	// to every declaration and report every such call as rewritten.
	//
	// The same Derive gives the masking of the line as it ran, which a
	// rewriting hook may have changed from the declaration's, and the
	// fields that decide whether its output is read (below).
	var s shape.Shape
	if len(pl.ToolInput) > 0 {
		s = shape.Derive(pl.ToolName, pl.ToolInput, p.st.Key())
		rec.ExecutedDigest = s.Digest
		rec.StatusMasked, rec.RunnerDigest, rec.RunnerKind = s.StatusMasked, s.RunnerDigest, s.RunnerKind
		if pl.HookEventName != FailureEvent {
			p.landedWrite = writtenCall{pl.ToolName, pl.ToolInput, s.VerbClass == shape.VerbWrite, s.MayWrite}
		}
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
	// A gate run's output, read, classified and dropped: three closed words
	// survive (card GATEOUT-WIRE1). Every other call's output is never read,
	// and its gate_output stays null.
	if shape.GateRun(s.RunnerDigest, s.StatusMasked, s.VerbClass) {
		out := gateOutput(payload, pl.HookEventName == FailureEvent, gateKind(&s), programOf(&s), start.Add(gateOutputBudget), p.now)
		rec.GateOutput = &out
	}
	// The content of the file a named write names, as the call left it: the
	// tree a later gate run is bound to (card P5A, D140). A digest of content,
	// so it is read only where record_scope v5 keeps it.
	scope := recordScope(p.st)
	if scope == store.RecordScopeV5 {
		rec.ContentSHA256 = writtenSHA(&pl)
	}
	// The record-scope gate, then the caps, as on the declaration path
	// (store.GateExecution; REC-52).
	store.GateExecution(&rec, scope)
	if p.capExec(&rec) {
		p.withheld = true
	}
	seq, err := p.st.LandExecution(rec)
	p.landedSeq, p.landedRunner, p.landedStore = seq, rec.RunnerDigest != nil, storeNamed(pl.ToolInput, p.st.Root())
	return landed(err, &p.withheld)
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
// It reads two presence bits on spans, true when a key carries anything but
// null or false, and the task id is never copied. stdout and stderr are not
// touched here: the one reader of those is gateOutput, for a gate run only. A
// response that is not an object is a foreground call: the flag is set only
// on evidence.
func backgroundedCall(in *hookio.Payload) bool {
	var bits [2]jsonscan.Span
	if jsonscan.Field(in.Raw, in.Spans[hookio.ToolResponse], backgroundKeys, bits[:]) != nil {
		return false
	}
	return bits[0].Set() || bits[1].Set()
}

// backgroundKeys are the two keys backgroundedCall reads, matched as the
// struct that once decoded them matched them: in any case.
var backgroundKeys = jsonscan.NewFoldKeySet("backgroundTaskId", "backgroundedByUser")

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
