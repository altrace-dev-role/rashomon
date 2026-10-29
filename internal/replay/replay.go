// Package replay turns one Claude Code transcript into the timeline of hook
// events the live recorder would have received while that session ran, each
// with a synthesized payload shaped as the recorder's input
// (docs/autosteer-prescan.md, B2a).
//
// The prescan imports history by replaying it through the live recorder
// (internal/hook: hook for PreToolUse, post for PostToolUse and
// PostToolUseFailure, and the Stop handling), so that what imported history
// means is exactly what live recording means. Nothing here derives a record:
// this package only rebuilds the input. B2b feeds the events to the recorder,
// into a store of its own, and compares what comes out with what a live run
// recorded.
//
// # What an event is
//
// Five kinds, the five the recorder acts on (install.Events lists eight; see
// "Which events are out of scope" for the other three):
//
//   - UserPromptSubmit, at the prompt's line;
//   - PreToolUse, at the call's tool_use block (history.Call.UsePos);
//   - PostToolUse or PostToolUseFailure, at the tool_result block that
//     answered it (history.Call.ResultPos);
//   - Stop, at the end of its turn (see "Where a turn ends").
//
// Events are sorted by position in the file, never by call. Calls overlap
// (parallel tool uses, a result that comes after the next prompt), and the
// order history.Read emits calls in, which is use order, is the right order
// for identity and the wrong one for time. A PreToolUse never crosses a
// result or a Stop that precedes it in the file, a Stop's predecessors are
// exactly the events before it, and a result after a Stop comes after that
// Stop (ORDER-1: a point sees exactly what is before it, and no point ever
// sees a later result).
//
// # Where a turn ends
//
// A turn is a genuine prompt and everything up to the next one. A Stop is
// established, and placed, only by the transcript's own account of the model:
// the main agent's last assistant message of the turn says stop_reason
// "end_turn", no interrupt marker and no line that could not be read follows
// it, and the turn did not go on with another message. The Stop sits at the
// end of that message's last line, after every block on it, so the events
// before it are the ones on earlier lines. A late result, one on a line after
// the last assistant line, comes after the Stop, as it did live.
//
// The rule uses nothing a hook wrote. The system line Claude Code writes after
// a Stop hook ran (stop_hook_summary) is not read, and removing it, and every
// attachment carrying a hook's output, moves no Stop, because history from
// before Rashomon was installed has none (TestREPLAY3_StopBoundariesDoNotNeedHookLines
// runs both fixtures both ways).
//
// Where the transcript does not establish a Stop, the event is a Stop of
// unknown boundary (Event.BoundaryUnknown), with a reason, placed at the end of
// the turn's last line. It is not evaluated as a Stop and it is a Gap. These
// are the cases, and the file ending is none of them by itself:
//
//   - the file ends mid-turn, after a tool_result, a tool_use or the prompt
//     (StopNotEndTurn), as a killed or still running session's does;
//   - the last message stopped for another reason: max_tokens, tool_use
//     (StopNotEndTurn);
//   - an "[Request interrupted by user" line follows the last assistant
//     line (StopInterrupted). Claude Code fires neither Stop event after an
//     interrupt (install.EventUserPromptSubmit); the marker string is not in
//     a fixture, so the doubt is kept;
//   - a line that could not be read follows it (StopUnreadableLine);
//   - another assistant message follows an end_turn one in the same turn
//     (StopContinued), as when a Stop hook blocked the stop: a Stop may have
//     fired at the earlier end, and only the hook's own line would say so.
//
// A prompt followed by another with no assistant line between them and every
// line readable is established to have had no model turn, and gets no event.
// Turn records every turn's end for the coverage report.
//
// # Gaps and taint
//
// A skipped event is never skipped silently. An event that is Unsupported, a
// Stop whose boundary is unknown, a PreToolUse whose result history gave up on
// (Evicted), and a line that could not be read each leave evidence incomplete
// from their position on. Every event carries Taint, the earliest such position
// at or before it (zero while the evidence is complete), and Summary.TaintFrom
// says where the whole timeline's began. B2b must not call a verdict at or after
// Taint Verified, since the skipped event could have changed it. A denial
// and a call with no result at the end of the file are not gaps: both are what
// a live run recorded.
//
// # Which events are out of scope
//
// install.Events is PreToolUse, PostToolUse, PostToolUseFailure,
// SessionStart, SessionEnd, Stop, StopFailure and UserPromptSubmit. There is
// no SubagentStop in it, so a subagent's turn end is not an event, and a
// sidechain line never moves a turn's end. SessionStart and SessionEnd are
// the liveness probe: they write a coverage marker, not evidence, and a
// transcript records no moment for either, so they are not rebuilt. StopFailure
// fires where an API error ends the turn in place of Stop; a transcript
// records the error as a line this reader has no fixture for, so a turn that
// ended that way is one of the unknown-Stop cases above rather than a guess at
// StopFailure's payload.
//
// # What is never guessed
//
// A payload carries the fields the recorder's entry points read for its event
// (Fields is the table, with the evidence for each row), each taken from the
// transcript. A field the recorder reads and the transcript cannot supply
// makes that event Unsupported, with a named Reason, and no payload: the field
// is not filled with a plausible value. Fields nothing reads are not rebuilt
// (two are, for shape, and equal), so the prompt's text never enters the
// timeline, and the tool's output enters it only as the head of an error
// message, which is all PostToolUseFailure's error field is read for. One
// field is read, stored and omitted by design: duration_ms (class Omitted).
//
// Nothing here stores anything. An event's payload lives until the caller's
// yield returns, unless the caller keeps it (Build does).
package replay

import (
	"container/heap"
	"errors"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// Kind is the hook event an Event stands for. Its String is the
// hook_event_name the payload carries.
type Kind uint8

const (
	// UserPromptSubmit is the recap catch-up's event: `recap pending`.
	UserPromptSubmit Kind = iota + 1
	// PreToolUse is the declaration's event: `hook`.
	PreToolUse
	// PostToolUse is the execution's event for a call that did not fail:
	// `post`.
	PostToolUse
	// PostToolUseFailure is the execution's event for a call that failed;
	// `post` serves both.
	PostToolUseFailure
	// Stop is the end of a turn: `recap`.
	Stop
)

var kindNames = [...]string{"", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Stop"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "Kind(?)"
}

// Reason is why an event is Unsupported: the transcript cannot supply a field
// the recorder reads, or what it does supply is not a shape this program has
// a fixture for. Every Reason names one cause.
type Reason string

const (
	// NoVersion, UnsupportedVersion and ResultConflict are history's:
	// the line names no Claude Code version, a version with no fixture, or
	// two results for one call that disagree.
	NoVersion          Reason = "no_version"
	UnsupportedVersion Reason = "unsupported_version"
	ResultConflict     Reason = "result_conflict"
	// NoSessionID: the line names no session. The recorder files a payload
	// that names none under an unattributed session, which a replay is not.
	NoSessionID Reason = "no_session_id"
	// NoTranscriptPath: the caller gave no path for the file, and the payload
	// names the transcript it came from.
	NoTranscriptPath Reason = "no_transcript_path"
	// NoCWD and NoResultCWD: the tool_use line (or the prompt's, or the turn's
	// last) names no working directory, or the tool_result line names none or
	// one over history's bound.
	NoCWD       Reason = "no_cwd"
	NoResultCWD Reason = "no_result_cwd"
	// NoPromptID: no prompt is known for the event. A tool_use before the
	// file's first prompt, or a result line with no promptId.
	NoPromptID Reason = "no_prompt_id"
	// NoPermissionMode: the turn's prompt line names no permissionMode.
	NoPermissionMode Reason = "no_permission_mode"
	// PermissionModeChanged: a permission-mode line falls in the turn before
	// the event, so the mode the prompt was sent under may not be the mode the
	// call ran under. No fixture holds such a line, so this is detected by its
	// type name alone, and unverified.
	PermissionModeChanged Reason = "permission_mode_changed_mid_turn"
	// SidechainAgentType: a subagent's call. The payload names the subagent's
	// agent_id and agent_type, which the recorder stores, and a transcript
	// names the first only.
	SidechainAgentType Reason = "sidechain_agent_type"
	// InputDropped: the tool_use input was over history's bound, or absent.
	InputDropped Reason = "tool_input_dropped"
	// InterruptedCall: the call was interrupted. No fixture holds one, so the
	// payload a PostToolUseFailure carries for it (is_interrupt, error) is not
	// known.
	InterruptedCall Reason = "interrupted_payload_unmeasured"
	// ResponseUnreadable: a Bash call whose result's toolUseResult cannot be
	// read as an object, so whether tool_response carried backgroundTaskId or
	// backgroundedByUser is not known (hook.backgroundedCall reads both).
	ResponseUnreadable Reason = "tool_response_unreadable"
	// BackgroundUnconfirmed: a Bash call launched with run_in_background whose
	// result names neither key. The live payload measured on 2.1.280 does; the
	// transcript's toolUseResult may not mirror it for a background launch, and
	// no fixture says.
	BackgroundUnconfirmed Reason = "background_unconfirmed"
	// PromptShape: a user line that starts a turn but is not a plain-string
	// prompt, the one shape in the fixtures.
	PromptShape Reason = "prompt_shape_unverified"
	// NoFinalText, SplitFinalMessage and FinalTextTooLarge: the Stop's
	// last_assistant_message cannot be taken from the end line: it has no
	// text block; the message holds more than one text block, so how Claude
	// Code joins them is not known; or the text is over the bound.
	NoFinalText       Reason = "no_final_text"
	SplitFinalMessage Reason = "final_message_text_unmeasured"
	FinalTextTooLarge Reason = "final_message_too_large"
	// The Stop boundary is unknown: the transcript alone does not establish
	// that a Stop fired here. StopInterrupted: the turn's last assistant line
	// is followed by an interrupt marker. StopNotEndTurn: the last assistant
	// message stopped for another reason, or the turn has no assistant
	// message and the file ends (cut mid-turn, killed, or still running).
	// StopUnreadableLine: a line that could not be read follows the last
	// assistant message. StopContinued: the turn went on after an end_turn
	// message, as it does when another Stop hook blocked the stop.
	StopInterrupted    Reason = "stop_unknown_interrupted_turn"
	StopNotEndTurn     Reason = "stop_unknown_last_message_not_end_turn"
	StopUnreadableLine Reason = "stop_unknown_unreadable_line_after_last_message"
	StopContinued      Reason = "stop_unknown_turn_continued_after_end_turn"
	// Evicted is the Gap of a PreToolUse whose result history gave up on to
	// stay within its limits: the result is in the file and not in the replay.
	Evicted Reason = "evicted"
)

// Event is one hook invocation the live recorder would have received.
type Event struct {
	Kind Kind
	// Pos is where in the file the event happened.
	Pos history.Pos
	// SessionID is the sessionId of the line the event is taken from.
	SessionID string
	// The call-bearing events (Pre and Post) name the call. Seq is the call's
	// place in use order, -1 on the others.
	ToolUseID string
	ToolName  string
	Seq       int
	// Grade and GradeReason are history's, for the call.
	Grade       history.Grade
	GradeReason history.Reason
	// Denied: a PreToolUse for a call a confirmed denial refused
	// (report.IsDenial). No PostToolUse follows it, as none did live.
	Denied bool
	// Missing: a PreToolUse for a call whose result was not read: the file ends
	// first, or the call was evicted (history.Limits). No Post follows it
	// either, and it is not a denial: nothing says the call was refused.
	Missing bool
	// Unsupported lists why the event cannot be fed to the recorder, in the
	// order found. Payload is nil when it is not empty.
	Unsupported []Reason
	// BoundaryUnknown: a Stop whose place the transcript does not establish
	// (see "Where a turn ends"). Its Pos is the end of the turn's last line
	// and Unsupported names why. It is not a Stop to be evaluated; it is a
	// point after which evidence is incomplete.
	BoundaryUnknown bool
	// Gap is why this event leaves evidence incomplete from its position on,
	// empty when it does not: the first of Unsupported, or Evicted for a call
	// whose result history dropped to stay within its limits. A skipped event
	// is never skipped silently: it is a Gap.
	Gap Reason
	// Taint is the earliest position at or before this event where evidence
	// became incomplete: the first Gap event, or the first line that could not
	// be read, whichever the file has first. The zero Pos means every event
	// up to and including this one is complete. B2b must not call a verdict
	// at a position from Taint on Verified, since the skipped event could have
	// changed it.
	Taint history.Pos
	// Payload is the JSON object the recorder would have read on stdin.
	Payload []byte
}

// Supported reports whether the event has a payload to feed.
func (e *Event) Supported() bool { return len(e.Unsupported) == 0 }

// EndKind is how a turn ended.
type EndKind uint8

const (
	// EndStop: the turn ended with the main agent's end_turn, and a Stop event
	// stands at its end (supported or not).
	EndStop EndKind = iota
	// EndInterrupted: a "[Request interrupted by user" line follows the last
	// assistant line. No Stop.
	EndInterrupted
	// EndOpen: the last assistant message stopped for another reason, which
	// StopReason names ("tool_use" is a file cut mid-turn). No Stop.
	EndOpen
	// EndNoAssistant: the prompt was followed by no assistant line of the main
	// agent. No Stop.
	EndNoAssistant
)

var endNames = [...]string{"stop", "interrupted", "open", "no_assistant"}

func (k EndKind) String() string {
	if int(k) < len(endNames) {
		return endNames[k]
	}
	return "EndKind(?)"
}

// Turn is one prompt and how its turn ended, for the coverage report.
type Turn struct {
	// Prompt is where the prompt's line is; PromptID its promptId.
	Prompt   history.Pos
	PromptID string
	End      EndKind
	// EndPos is the end of the last assistant line (the Stop's position for
	// EndStop), zero for EndNoAssistant.
	EndPos history.Pos
	// StopReason is the last assistant message's stop_reason as written.
	StopReason string
}

// Summary is what Walk saw besides the events.
type Summary struct {
	Stats history.Stats
	Turns []Turn
	// TaintFrom is the earliest position evidence became incomplete in the
	// whole file, zero when it never did: the Taint of the last event, or of
	// a line that could not be read after it. Gaps counts the events that are
	// Gap, and Unreadable the lines that could not be read.
	TaintFrom  history.Pos
	Gaps       int
	Unreadable int
}

// Options configure a replay.
type Options struct {
	// Limits bound the call reader (history.Limits). The zero value reads with
	// history.DefaultLimits.
	Limits history.Limits
	// TranscriptPath is the path of the file being read, as the payloads name
	// it in transcript_path. It is the caller's, not the reader's: a reader
	// of one stream cannot know where it came from.
	TranscriptPath string
}

func (o Options) limits() history.Limits {
	if o.Limits == (history.Limits{}) {
		return history.DefaultLimits
	}
	return o.Limits
}

// Timeline is a transcript's events in file order.
type Timeline struct {
	Events []Event
	Summary
}

// ErrStop ends a Walk early. Walk returns nil for it.
var ErrStop = errors.New("replay: stop")

// Build reads src to the end and returns every event, in file order. It holds
// them all; Walk does not.
func Build(src io.ReadSeeker, opt Options) (Timeline, error) {
	var tl Timeline
	sum, err := Walk(src, opt, func(e Event) error {
		tl.Events = append(tl.Events, e)
		return nil
	})
	tl.Summary = sum
	return tl, err
}

// Walk reads src twice, from the start both times, and calls yield with each
// event in file order. It holds only the events that cannot yet be placed.
//
// The first pass reads the turns: where each prompt and each turn's end is
// (a few small records per turn, O(turns)). The second reads the calls
// (history.Read) and merges them with the turns' events. The merge is exact,
// not approximate: history emits calls in use order, so once a call with
// UsePos u is emitted, no call still to come has an event at or before u (its
// own use is after u, and its result after its use), and every event at or
// before u is in hand. The call events wait in a heap until then, so a call
// whose result is far from its use holds a small event, not the file.
//
// yield's error ends the walk; ErrStop ends it without one. An error from
// src is returned; every event before it has been yielded.
func Walk(src io.ReadSeeker, opt Options, yield func(Event) error) (Summary, error) {
	sc, err := scanTurns(src, opt)
	if err != nil {
		return Summary{}, err
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return Summary{}, err
	}
	w := &walker{opt: opt, sc: sc, yield: yield}
	stats, rerr := history.Read(src, opt.limits(), w.call)
	if w.err == nil {
		w.drain(nil)
	}
	// An unreadable line after the last event is still a gap.
	w.taint(history.Pos{Line: int(^uint(0) >> 1)})
	sum := Summary{Stats: stats, Turns: sc.turns, TaintFrom: w.first, Gaps: w.gaps, Unreadable: sc.unreadable}
	switch {
	case errors.Is(w.err, ErrStop):
		return sum, nil
	case w.err != nil:
		return sum, w.err
	}
	return sum, rerr
}

// walker merges the calls history emits with the turns' events.
type walker struct {
	opt   Options
	sc    *scan
	yield func(Event) error
	err   error
	heap  eventHeap
	next  int // the next of sc.events to place
	// first is the earliest position evidence became incomplete among the
	// events placed so far and the unreadable lines before them; gaps counts
	// the Gap events placed.
	first history.Pos
	gaps  int
}

// taint moves first back to the first unreadable line, if that line is
// before at and first is not already earlier.
func (w *walker) taint(at history.Pos) {
	u := w.sc.firstUnreadable
	if !u.IsZero() && u.Before(at) && (w.first.IsZero() || u.Before(w.first)) {
		w.first = u
	}
}

// call is history.Read's emit: the call's events join the heap, then
// everything at or before the call's use is placed. A call's use is the
// watermark: see Walk.
func (w *walker) call(c history.Call) {
	if w.err != nil {
		return
	}
	heap.Push(&w.heap, w.pre(&c))
	if ev := w.post(&c); ev != nil {
		heap.Push(&w.heap, ev)
	}
	w.drain(&c.UsePos)
}

// drain yields, in order, every event at or before the watermark from the
// heap and from the turns' events, or every event when it is nil.
func (w *walker) drain(through *history.Pos) {
	for w.err == nil {
		var e *Event
		fromHeap := false
		if len(w.heap) > 0 {
			e, fromHeap = w.heap[0], true
		}
		if w.next < len(w.sc.events) {
			if t := &w.sc.events[w.next]; e == nil || less(t, e) {
				e, fromHeap = t, false
			}
		}
		if e == nil || through != nil && through.Before(e.Pos) {
			return
		}
		if fromHeap {
			heap.Pop(&w.heap)
		} else {
			w.next++
		}
		// Placed in file order, so every event sees the gaps before it and
		// its own, and the earliest never moves later.
		w.taint(e.Pos)
		if e.Gap != "" {
			w.gaps++
			if w.first.IsZero() {
				w.first = e.Pos
			}
		}
		e.Taint = w.first
		w.err = w.yield(*e)
	}
}

// less orders events: by position, then by kind, then by call. Two events are
// never at one position in practice (a block is one tool_use or one
// tool_result, a prompt is block 0 of its line, a Stop is past the last block
// of its), and the tie-break makes the order total anyway.
func less(a, b *Event) bool {
	if a.Pos != b.Pos {
		return a.Pos.Before(b.Pos)
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Seq < b.Seq
}

type eventHeap []*Event

func (h eventHeap) Len() int           { return len(h) }
func (h eventHeap) Less(i, j int) bool { return less(h[i], h[j]) }
func (h eventHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)        { *h = append(*h, x.(*Event)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return x
}
