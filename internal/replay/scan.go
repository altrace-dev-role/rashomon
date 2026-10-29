package replay

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/history"
	"github.com/altrace-dev-role/rashomon/internal/linescan"
	"github.com/altrace-dev-role/rashomon/internal/report"
)

// maxFinalTextBytes bounds the text kept for a Stop's last_assistant_message.
// The recorder reads the head of a message up to 256 KiB and its tail up to
// 64 KiB (cmd/rashomon messageHeadBytes, messageTailBytes), and the whole is
// refused past hookio.MaxPayloadBytes; a message over this is not kept and
// its Stop is Unsupported rather than cut, because a cut message is a
// different message.
const maxFinalTextBytes = 1 << 20

// interruptMarker opens the text of the user line Claude Code writes when the
// user interrupts a turn ("[Request interrupted by user]" and "...for tool
// use]"). It is not a prompt, and after the last assistant line it means the
// turn ended by interrupt, where neither Stop event fires
// (install.EventUserPromptSubmit).
const interruptMarker = "[Request interrupted by user"

// scan is the first pass: the turns, and the events that stand at a prompt
// or at a turn's end. Those are the events no call carries, and there are two
// per turn at most.
type scan struct {
	// events are the UserPromptSubmit and Stop events, in file order.
	events []Event
	turns  []Turn
	// ctx has one entry per prompt, by line, for finding what turn a line is in.
	ctx []turnCtx
	// firstUnreadable is the first line that could not be read (too long, or
	// not decodable), zero for none; unreadable counts them. Evidence is
	// incomplete from the first on: any such line may have been a prompt, a
	// result or a turn's end. Taint only ever moves earlier, so the first is
	// all a walker needs.
	firstUnreadable history.Pos
	unreadable      int
}

// turnCtx is what a call event needs from the turn it falls in: the prompt's
// id and permission mode, which a tool_use line does not carry, and where, if
// anywhere, the mode may have changed.
type turnCtx struct {
	line     int
	promptID string
	mode     string
	// permChange is the line of the first permission-mode line inside the
	// turn, 0 for none.
	permChange int
}

// at is the turn a line falls in: that of the last prompt before it. nil
// before the file's first prompt.
func (sc *scan) at(line int) *turnCtx {
	i := sort.Search(len(sc.ctx), func(i int) bool { return sc.ctx[i].line >= line })
	if i == 0 {
		return nil
	}
	return &sc.ctx[i-1]
}

// head is the part of a line the first pass decodes. Everything but the
// message's content is a key of the line itself.
type head struct {
	Type           string `json:"type"`
	IsSidechain    bool   `json:"isSidechain"`
	IsMeta         bool   `json:"isMeta"`
	PromptID       string `json:"promptId"`
	SessionID      string `json:"sessionId"`
	CWD            string `json:"cwd"`
	Version        string `json:"version"`
	PermissionMode string `json:"permissionMode"`
	Message        struct {
		ID         string          `json:"id"`
		StopReason string          `json:"stop_reason"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

// textBlock is a content block read for its type and, for a text block, its
// text. A tool_result's content is a key other than text and is not read.
type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// final is the main agent's last assistant message so far in the current
// turn: its lines are consecutive (Claude Code writes each content block of
// one API response as a line of its own, all with the response's id and
// stop_reason), and the last of them is where a Stop would stand.
type final struct {
	id         string
	stopReason string
	// end is just past the last block of the message's last line.
	end                     history.Pos
	sessionID, cwd, version string
	// texts counts the message's non-blank text blocks; text is the first
	// one's, unless it was over the bound (big).
	texts int
	text  string
	big   bool
}

// open is the turn being read.
type open struct {
	ctx    int // index into scan.ctx
	prompt Turn
	msg    *final
	// last is just past the last block of the last main-agent line of the
	// turn, whatever it was: where a Stop of unknown place is put. lastSession
	// is that line's session.
	last        history.Pos
	lastSession string
	// interrupted: an interrupt marker came after the last assistant line.
	interrupted bool
	// unreadable: a line that could not be read came after it, and may have
	// been the real end, or a later assistant line.
	unreadable bool
}

// scanTurns is the first pass.
func scanTurns(src io.Reader, opt Options) (*scan, error) {
	sc := &scan{}
	rd := linescan.New(src, report.TranscriptInitialBuffer, report.TranscriptMaxLine)
	var cur *open
	n := 0
	unreadable := func() {
		sc.unreadable++
		if sc.firstUnreadable.IsZero() {
			sc.firstUnreadable = history.Pos{Line: n}
		}
	}
	for rd.Scan() {
		n++
		if rd.TooLong() {
			unreadable()
			if cur != nil {
				cur.unreadable = true
			}
			continue
		}
		var h head
		switch report.DecodeHeader(rd.Bytes(), &h) {
		case report.LineBlank:
			continue
		case report.LineUndecodableSidechain:
			// A subagent's line never starts or ends the main agent's turn,
			// but history counts it unread and so does the taint.
			unreadable()
			continue
		case report.LineUndecodable:
			unreadable()
			if cur != nil {
				cur.unreadable = true
			}
			continue
		}
		if h.Type == "permission-mode" {
			if cur != nil && sc.ctx[cur.ctx].permChange == 0 {
				sc.ctx[cur.ctx].permChange = n
			}
			continue
		}
		if h.IsSidechain {
			// SubagentStop is not an installed event, and a sidechain line
			// never moves a turn's end.
			continue
		}
		switch h.Type {
		case "user":
			kind, blocks := userKind(&h)
			switch kind {
			case userPrompt, userPromptOther:
				sc.finish(cur, opt, false)
				cur = sc.start(&h, n, kind == userPromptOther, opt)
			case userInterrupt:
				if cur != nil && cur.msg != nil {
					cur.interrupted = true
				}
			}
			if cur != nil {
				cur.seen(&h, n, blocks)
			}
		case "assistant":
			if cur != nil {
				sc.assistant(cur, &h, n)
			}
		}
	}
	if err := rd.Err(); err != nil {
		return nil, err
	}
	sc.finish(cur, opt, true)
	return sc, nil
}

type userLine int

const (
	userOther userLine = iota // a tool result, a meta line, anything that starts no turn
	// userPrompt: the line's content is a plain string, the one shape of a
	// typed prompt in the fixtures.
	userPrompt
	// userPromptOther: a user line that is not a tool result, a meta line or
	// an interrupt, with any other content: a prompt whose shape no fixture
	// shows (blocks of text and image).
	userPromptOther
	userInterrupt
)

// userKind classifies a main-agent user line and counts its blocks. The rule
// for a prompt is the report's (FinalAssistantTexts): not a meta line, and not
// an array of tool_result blocks and nothing else; here, not an array holding
// one at all, since a result with text beside it is still a call finishing.
func userKind(h *head) (userLine, int) {
	c := h.Message.Content
	if len(c) == 0 {
		return userOther, 1
	}
	if c[0] == '"' {
		if h.IsMeta {
			return userOther, 1
		}
		return userPrompt, 1
	}
	var blocks []textBlock
	if json.Unmarshal(c, &blocks) != nil || len(blocks) == 0 {
		return userOther, 1
	}
	if h.IsMeta {
		return userOther, len(blocks)
	}
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return userOther, len(blocks)
		}
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.HasPrefix(b.Text, interruptMarker) {
			return userInterrupt, len(blocks)
		}
	}
	return userPromptOther, len(blocks)
}

// seen notes a main-agent line as the turn's latest.
func (o *open) seen(h *head, n, blocks int) {
	o.last, o.lastSession = history.Pos{Line: n, Block: max(blocks, 1)}, h.SessionID
}

// start opens a turn at a prompt line and places its UserPromptSubmit.
func (sc *scan) start(h *head, n int, otherShape bool, opt Options) *open {
	sc.ctx = append(sc.ctx, turnCtx{line: n, promptID: h.PromptID, mode: h.PermissionMode})
	pos := history.Pos{Line: n}
	ev := Event{Kind: UserPromptSubmit, Pos: pos, SessionID: h.SessionID, Seq: -1}
	un := &unsupported{ev: &ev}
	un.version(h.Version)
	un.when(h.SessionID == "", NoSessionID)
	un.when(opt.TranscriptPath == "", NoTranscriptPath)
	un.when(h.CWD == "", NoCWD)
	un.when(otherShape, PromptShape)
	if ev.Supported() {
		ev.Payload = marshal(&recapPayload{SessionID: h.SessionID, TranscriptPath: opt.TranscriptPath, CWD: h.CWD,
			HookEventName: UserPromptSubmit.String()})
	}
	ev.settle()
	sc.events = append(sc.events, ev)
	return &open{ctx: len(sc.ctx) - 1, prompt: Turn{Prompt: pos, PromptID: h.PromptID}}
}

// assistant reads a main-agent assistant line into the turn's last message.
// A new message after an end_turn one means the turn went on past a point
// where it could have stopped: another Stop hook blocked the stop, or
// something else restarted the model. A Stop may have fired at that earlier
// end, and the transcript without the hook's own line cannot say, so the
// earlier end is an unknown Stop.
func (sc *scan) assistant(o *open, h *head, n int) {
	var blocks []textBlock
	c := h.Message.Content
	switch {
	case len(c) > 0 && c[0] == '"':
		var s string
		if json.Unmarshal(c, &s) == nil {
			blocks = []textBlock{{Type: "text", Text: s}}
		}
	default:
		_ = json.Unmarshal(c, &blocks)
	}
	if m := o.msg; m != nil && m.id != h.Message.ID && m.stopReason == "end_turn" {
		sc.unknownStop(m.end, m.sessionID, StopContinued)
	}
	if o.msg == nil || o.msg.id != h.Message.ID {
		o.msg = &final{id: h.Message.ID}
	}
	m := o.msg
	m.stopReason = h.Message.StopReason
	m.end = history.Pos{Line: n, Block: max(len(blocks), 1)}
	m.sessionID, m.cwd, m.version = h.SessionID, h.CWD, h.Version
	for _, b := range blocks {
		if b.Type != "text" || strings.TrimSpace(b.Text) == "" {
			continue
		}
		if m.texts++; m.texts == 1 {
			if len(b.Text) > maxFinalTextBytes {
				m.big = true
			} else {
				m.text = b.Text
			}
		}
	}
	o.interrupted, o.unreadable = false, false
	o.seen(h, n, len(blocks))
}

// unknownStop places a Stop whose boundary the transcript does not establish.
func (sc *scan) unknownStop(at history.Pos, sessionID string, why Reason) {
	ev := Event{Kind: Stop, Pos: at, SessionID: sessionID, Seq: -1, BoundaryUnknown: true, Unsupported: []Reason{why}}
	ev.settle()
	sc.events = append(sc.events, ev)
}

// finish closes a turn: it records how the turn ended and places what stands
// at its end. last is whether the file ended with it.
//
// A Stop is placed only where the transcript establishes it: the main agent's
// last assistant message says end_turn, and nothing after it is an interrupt
// or a line that could not be read. Every other end is either established to
// have no Stop (the prompt was followed by another with no assistant line
// between, and every line was readable) or an unknown Stop. The file simply
// ending is not a Stop: a transcript cut after a tool_result, or one still
// being written, ends the same way.
func (sc *scan) finish(o *open, opt Options, last bool) {
	if o == nil {
		return
	}
	t := o.prompt
	m := o.msg
	switch {
	case m == nil:
		t.End = EndNoAssistant
		if last || o.unreadable {
			sc.unknownStop(o.last, o.lastSession, StopNotEndTurn)
		}
	case o.interrupted:
		t.End, t.EndPos, t.StopReason = EndInterrupted, m.end, m.stopReason
		sc.unknownStop(o.last, o.lastSession, StopInterrupted)
	case m.stopReason != "end_turn":
		t.End, t.EndPos, t.StopReason = EndOpen, m.end, m.stopReason
		sc.unknownStop(o.last, o.lastSession, StopNotEndTurn)
	case o.unreadable:
		t.End, t.EndPos, t.StopReason = EndOpen, m.end, m.stopReason
		sc.unknownStop(o.last, o.lastSession, StopUnreadableLine)
	default:
		t.End, t.EndPos, t.StopReason = EndStop, m.end, m.stopReason
		ev := Event{Kind: Stop, Pos: m.end, SessionID: m.sessionID, Seq: -1}
		un := &unsupported{ev: &ev}
		un.version(m.version)
		un.when(m.sessionID == "", NoSessionID)
		un.when(opt.TranscriptPath == "", NoTranscriptPath)
		un.when(m.cwd == "", NoCWD)
		un.when(m.texts == 0, NoFinalText)
		un.when(m.texts > 1, SplitFinalMessage)
		un.when(m.big, FinalTextTooLarge)
		if ev.Supported() {
			ev.Payload = marshal(&recapPayload{SessionID: m.sessionID, TranscriptPath: opt.TranscriptPath, CWD: m.cwd,
				HookEventName: Stop.String(), LastAssistantMessage: &m.text})
		}
		ev.settle()
		sc.events = append(sc.events, ev)
	}
	sc.turns = append(sc.turns, t)
}
