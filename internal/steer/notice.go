package steer

import "github.com/altrace-dev-role/rashomon/internal/store"

// The once-per-session notices (card VERSION-GATE1). A note shadowed because
// its channel is not verified for the client, and a held-out note, print
// nothing of the note; the first of each in a session prints one user-only
// line instead, so the user learns once that notes were built and why none
// reached the agent. A notice prints only on a call that prints nothing
// else, since a call has one systemMessage; otherwise the next call that
// qualifies prints it. The store's marker makes it once per session
// (store.TakeNotice), and the line it stands in for records it
// (Line.Notice), so nothing prints that no line records (STR-8).

// surfaceSDK is harness.SurfaceSDK's value, the surface of claude -p and the
// SDKs, which have no in-band mirror; TestVG1d checks the two agree.
const surfaceSDK = "sdk"

// holdoutNotice is NoticeHoldout's line.
const holdoutNotice = "rashomon: this session is in the measurement holdout: its notes are built and logged, " +
	"not sent to the agent. Shown once per session."

// marker is the store's marker file for n.
func (n Notice) marker() string {
	if n == NoticeHoldout {
		return store.FileNoticeHoldout
	}
	return store.FileNoticeChannel
}

// noticeFor is the line a notice would stand in for, and which notice: the
// first line, in decide's order, that unverified_channel shadowed or the
// holdout kept from the agent; nil when there is none. A call has at most
// one kind, since a held-out note was delivered, which a shadowed channel
// rules out.
func noticeFor(lines []*Line) (*Line, Notice) {
	for _, l := range lines {
		switch {
		case l.SuppressedBy != nil && *l.SuppressedBy == SuppressedByUnverifiedChannel:
			return l, NoticeChannel
		case l.Delivery != nil && *l.Delivery == DeliveryHoldout:
			return l, NoticeHoldout
		}
	}
	return nil, ""
}

// notice is the call's once-per-session line, for a call whose lines print
// nothing else, and the marker it took; the zero Output and "" when no line
// qualifies, the session has had the line, or the event cannot print it. The
// text is built only once the marker is taken, so a session pays for it once.
func notice(ctx *Context, c Client, lines []*Line) (Output, string) {
	l, n := noticeFor(lines)
	if l == nil {
		return Output{}, ""
	}
	m := n.marker()
	if !ctx.Store.TakeNotice(ctx.SessionID, m) {
		return Output{}, ""
	}
	text := holdoutNotice
	if n == NoticeChannel {
		text = channelNotice(ctx.Event, c, ctx.Surface, mirrorVerifiedOn())
	}
	out := Output{SystemMessage: text}
	if _, err := out.MarshalFor(string(ctx.Event), c); err != nil {
		ctx.Store.ReturnNotice(ctx.SessionID, m)
		return Output{}, ""
	}
	l.Notice = &n
	return out, m
}

// channelNotice is NoticeChannel's line for c on event under table: the
// Claude Code version, the event and its floor, that the note was built and
// not sent to the agent, and why (shutBy). Nothing of the note itself. A
// version that is not dotted digits is not printed.
func channelNotice(event Event, c Client, surface string, table map[string]channelBasis) string {
	e := string(event)
	b := table[e]
	version := "Claude Code " + c.Version
	if !digitRuns(c.Version, '.') {
		version = "Claude Code, version unreadable"
	}
	floor := "delivery verified from " + b.floor
	if b.floor == "" {
		floor = "delivery verified on no version yet"
	}
	var why string
	switch shutBy(outputKeys()[e], e, c, table) {
	case shutNoMirror:
		why = "the session's surface could not be read, and notes go only where the user sees what the agent is told"
		if surface == surfaceSDK {
			why = "claude -p and SDK sessions receive no notes by design"
		}
	case shutSubagent:
		why = "notes on a subagent's calls are not sent yet"
	case shutEventClosed:
		why = "notes on this event wait for a release that passes the delivery check"
	case shutUnknownVersion:
		why = "the version could not be read"
	case shutBelowFloor:
		why = "this version is older"
	case shutExcluded:
		why = "this version failed the delivery check"
	default:
		// Unreachable: a line is unverified_channel only where ContextAllowed,
		// which is shutBy, refused.
		why = "the channel is not verified"
	}
	return "rashomon: a note was built and not sent to the agent (" + version + ", " + e + "; " + floor + "): " +
		why + ". Shown once per session."
}
