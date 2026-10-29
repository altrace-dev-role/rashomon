package steer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The per-prompt caps (tech 4.2.5): 3 notes per (session_id, prompt_id),
// subagents included, and 5 user flags printed per prompt, the rest on the
// turn-end line.
const (
	maxNotes = 3
	maxFlags = 5
)

// patternVersion is every line's pattern_version: the spec example's value,
// with no per-pattern version table yet (card 2.2 R8, D52).
const patternVersion = 1

// holdoutSuffix ends a held-out note's user text (tech 5.2.4 E, R2).
const holdoutSuffix = " (held out: not sent to the agent)"

// channelRule is what an event's row lets a note do (card 2.2 R4), read
// from Marshal's own key table so the router and the marshaller cannot
// disagree about a row (STR-10).
type channelRule uint8

const (
	// ruleNoChannel: nothing prints. PreToolUse's systemMessage needs a
	// Decision, and v0 issues none; an event outside the table fails closed.
	ruleNoChannel channelRule = iota
	// ruleContext: a note goes in additionalContext where the Client's
	// channel is verified (ContextAllowed), else it ships in shadow.
	ruleContext
	// ruleFlagOnly: the event has no model channel (Stop, PostModelSwitch),
	// so a note prints as a flag and the event is the reason.
	ruleFlagOnly
)

func ruleOf(e Event) channelRule {
	keys, ok := outputKeys()[string(e)]
	switch {
	case !ok || keys.decision:
		return ruleNoChannel
	case keys.context:
		return ruleContext
	}
	return ruleFlagOnly
}

// channelOf is the line's channel for an event: PostToolUseFailure has none
// of its own, post serving it beside PostToolUse (tech 4.2.1); an unknown
// event has none.
func channelOf(e Event) Channel {
	switch e {
	case EventPreToolUse:
		return ChannelPreToolUse
	case EventPostToolUse, EventPostToolUseFailure:
		return ChannelPostToolUse
	case EventPostToolBatch:
		return ChannelPostToolBatch
	case EventUserPromptSubmit:
		return ChannelUserPromptSubmit
	case EventSessionStart:
		return ChannelSessionStart
	case EventSubagentStart:
		return ChannelSubagentStart
	case EventStop:
		return ChannelStop
	case EventPostModelSwitch:
		return ChannelPostModelSwitch
	}
	return ""
}

// rank is step 8's order (tech 4.2.4): priority descending, then the lower
// after_seq, then input order, a stable sort in place. Step 8's "then steer
// id" is superseded: ids are minted after the sort, so they cannot order it
// (card 2.2 R5).
func rank(findings []Finding) []Finding {
	slices.SortStableFunc(findings, func(a, b Finding) int {
		if a.Priority != b.Priority {
			return b.Priority - a.Priority
		}
		switch {
		case a.AfterSeq < b.AfterSeq:
			return -1
		case a.AfterSeq > b.AfterSeq:
			return 1
		}
		return 0
	})
	return findings
}

// newID is a line id when the Context mints none: 32 hex characters from
// crypto/rand, the invocation id's form.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails (Go 1.24)
	return hex.EncodeToString(b[:])
}

// noteTexts is a note's model text and its mirror (tech 4.2.7: `※ rashomon
// told the agent: "<model text>"`, byte for byte; MirrorBriefing for the one
// multi-line note), or the reason neither may be written: a text over the
// steer log's cap is withheld (output_gate, G2), and one the renderer's
// whole-string check refuses is render_refused. Neither refusal is
// reachable through Route -- a Message comes only from render.New, already
// validated under 1,200 bytes, and its mirror is at most 1,229 bytes -- so
// both are tested here, by string (card 2.2 R7, A8).
func noteTexts(text string, briefing bool) (model, user string, refused *SuppressedBy) {
	if len(text) > store.MaxSteerTextBytes {
		return "", "", reason(SuppressedByOutputGate)
	}
	mirror := render.Mirror
	if briefing {
		mirror = render.MirrorBriefing
	}
	user, err := mirror(text)
	if err != nil {
		return "", "", reason(SuppressedByRenderRefused)
	}
	if len(user) > store.MaxSteerTextBytes {
		return "", "", reason(SuppressedByOutputGate)
	}
	return text, user, nil
}

// flagText is a flag's user text: the rendered text itself, never the
// mirror wrapper, which says the agent was told (card 2.2 R7, D50); over
// the cap it is withheld (output_gate).
func flagText(text string) (user string, refused *SuppressedBy) {
	if len(text) > store.MaxSteerTextBytes {
		return "", reason(SuppressedByOutputGate)
	}
	return text, nil
}

func reason(r SuppressedBy) *SuppressedBy { return &r }

// clampReason is the first clamp to flag that bites, in step order (tech
// 4.2.4 steps 2, 5 and 6; card 2.2 R3): steering off, consent not current,
// no prompt to charge (4.2.5: a missing budget counts as spent), and a Stop
// whose hook is already active.
func clampReason(ctx *Context) *SuppressedBy {
	switch {
	case ctx.SteerOff:
		return reason(SuppressedBySteerOff)
	case !ctx.Consent:
		return reason(SuppressedByConsentMissing)
	case ctx.PromptID == nil:
		return reason(SuppressedByBudgetPrompt)
	case ctx.Event == EventStop && ctx.StopHookActive:
		return reason(SuppressedByStopHookActive)
	}
	return nil
}

// plan is one admitted finding's decision while decide makes it.
type plan struct {
	f       Finding
	key     findingKey
	level   Level
	state   LineState
	by      *SuppressedBy
	slots   map[string]json.RawMessage
	model   string
	user    string
	decide  bool // still to be decided by the budget passes
	holdout bool // held out: the user got the note, the model nothing
}

func (p *plan) suppress(r SuppressedBy) {
	p.state, p.by, p.decide = LineStateSuppressed, reason(r), false
}

// holdoutEligible is the field holdout's eligible set (card 2.5 R3; tech
// 5.2.4 E): a note charge delivered, which is the call's one note. Every flag
// is ineligible, whether its finding is one, a clamp made it one
// (clampReason: steering off, no consent, no prompt, an active Stop hook) or
// its event has no model channel (ruleFlagOnly: Stop, PostModelSwitch); no
// stop form reaches decide, LevelStop being a nudge here (STR-20) and
// StopContinue never set; and a line that did not print has nothing to hold.
func holdoutEligible(p *plan) bool {
	return p.state == LineStateDelivered && p.level == LevelNudge
}

// holdOut gives the user the note's own text with holdoutSuffix and the
// model nothing; never the mirror, which says the agent was told (G1).
func (p *plan) holdOut() {
	p.user, p.model, p.holdout = p.model+holdoutSuffix, "", true
}

// decide is the router's pure core (card 2.2 R1): no clock, no randomness,
// no I/O. It turns the admitted findings, the prompt's counts read under
// the lock and the client c into the call's Output and one line per
// finding whose effective level is not off. One note and one flag print at
// most, and when a note prints the flag waits: a call has one systemMessage,
// and the note's mirror is the invariant (INV-5). Every line records why it
// did or did not print.
func decide(ctx *Context, c Client, admitted []Finding, have counts, mint func() string) (Output, []*Line) {
	arm := ctx.Arm
	if arm == "" {
		arm = ArmLive
	}
	attendance := ctx.Attendance
	if attendance == "" {
		attendance = AttendanceAttended
	}
	rule := ruleOf(ctx.Event)
	plans := make([]plan, 0, len(admitted))
	claimed := make(map[findingKey]bool, len(admitted))
	for _, f := range rank(admitted) {
		level := f.MaxLevel
		if level == LevelOff {
			continue // dropped: no line, no charge (step 5)
		}
		if level == LevelStop {
			level = LevelNudge // stop needs a project stop setting, and no config exists (STR-20)
		}
		p := plan{f: f, key: findingKey{string(f.Pattern), string(f.Kind), f.Key}, level: level, decide: true}
		if p.level == LevelNudge {
			if by := clampReason(ctx); by != nil {
				p.level, p.by = LevelFlag, by
			}
		}
		if p.level == LevelNudge && rule == ruleFlagOnly {
			p.level = LevelFlag // the event has no model channel; the event says why (D50)
		}
		slots, slotErr := slotsOf(f.Message)
		p.slots = slots
		switch {
		case arm == ArmOff:
			p.suppress(SuppressedByArmOff)
		case rule == ruleNoChannel:
			p.suppress(SuppressedByNoChannel)
		case claimed[p.key]:
			p.suppress(SuppressedByBudgetPattern)
		case f.Message.Text() == "" || slotErr != nil:
			p.suppress(SuppressedByRenderRefused)
		case p.level == LevelNudge && !ContextAllowed(string(ctx.Event), c):
			p.suppress(SuppressedByUnverifiedChannel)
		case p.level == LevelNudge && attendance == AttendanceUnattended:
			p.suppress(SuppressedByNoMirror) // ED-9: the mirror has no reader (D51)
		}
		claimed[p.key] = true
		if p.decide {
			var refused *SuppressedBy
			if p.level == LevelNudge {
				p.model, p.user, refused = noteTexts(f.Message.Text(), f.Pattern == pattern.ContinuityBriefing)
			} else {
				p.user, refused = flagText(f.Message.Text())
			}
			if refused != nil {
				p.suppress(*refused)
			}
		}
		plans = append(plans, p)
	}

	note, flag := charge(plans, have)
	for i := range plans {
		if ctx.Holdout && holdoutEligible(&plans[i]) {
			plans[i].holdOut()
		}
	}
	var out Output
	switch {
	case note >= 0:
		out = Output{AdditionalContext: plans[note].model, SystemMessage: plans[note].user}
	case flag >= 0:
		out = Output{SystemMessage: plans[flag].user}
	}
	if out != (Output{}) {
		if _, err := out.MarshalFor(string(ctx.Event), c); err != nil {
			for i := range plans {
				if plans[i].state == LineStateDelivered {
					plans[i].suppress(SuppressedByOutputGate)
				}
			}
			out = Output{}
		}
	}

	lines := make([]*Line, len(plans))
	for i := range plans {
		lines[i] = lineOf(ctx, arm, attendance, &plans[i], mint())
	}
	return out, lines
}

// charge is the budget's two passes over the plans still to decide, in rank
// order (card 2.2 R6): a note whose key the prompt already delivered is
// budget_pattern, the first note the prompt can afford delivers, and a later
// one in the same call is superseded (budget_batch); a flag delivers only in
// a call where no note did, one per call under the prompt's cap, and every
// other flag is a candidate for the turn-end line. It returns the indexes
// that print, -1 for none.
func charge(plans []plan, have counts) (note, flag int) {
	note, flag = -1, -1
	for i := range plans {
		p := &plans[i]
		if !p.decide || p.level != LevelNudge {
			continue
		}
		switch {
		case have.keys[p.key]:
			p.suppress(SuppressedByBudgetPattern)
		case note >= 0:
			p.state, p.by, p.decide = LineStateSuperseded, reason(SuppressedByBudgetBatch), false
		case have.notes >= maxNotes:
			p.suppress(SuppressedByBudgetPrompt)
		default:
			p.state, p.decide, note = LineStateDelivered, false, i
		}
	}
	for i := range plans {
		p := &plans[i]
		if !p.decide {
			continue
		}
		if note >= 0 || flag >= 0 || have.flags >= maxFlags {
			p.state, p.decide = LineStateCandidate, false // the turn-end line's, 2.4
			continue
		}
		p.state, p.decide, flag = LineStateDelivered, false, i
	}
	return note, flag
}

// lineOf is a decided plan's steer line (card 2.2 R8). Text is kept only on
// a line that printed (RES-32): every other line keeps its slots and
// renderer_version and is re-rendered on read.
func lineOf(ctx *Context, arm Arm, attendance Attendance, p *plan, id string) *Line {
	l := &Line{
		Type: LineType, V: LineVersion, ID: id, SteerID: id, AtUnixMS: ctx.NowUnixMS,
		SessionID: ctx.SessionID, Project: ctx.Project,
		Pattern: string(p.f.Pattern), Kind: string(p.f.Kind), Key: p.f.Key, PatternVersion: patternVersion,
		Level: p.level.String(), Channel: channelOf(ctx.Event), Event: string(ctx.Event),
		Attendance: attendance, Arm: arm, AfterSeq: p.f.AfterSeq,
		EvidenceSeqs: append(make([]int64, 0, len(p.f.Evidence)), p.f.Evidence...),
		Slots:        p.slots, State: p.state, SuppressedBy: p.by, InvocationID: ctx.InvocationID,
		RendererVersion: render.Version, RulesVersion: shape.RulesVersion,
		Delivery:  DeriveDelivery(p.state, p.level, p.by, false),
		HarnessID: ctx.HarnessID, Surface: ctx.Surface, Mirror: MirrorNone,
	}
	if p.holdout && p.state == LineStateDelivered {
		d := DeliveryHoldout // recorded by the draw, never derived (tech 5.2.4 E)
		l.Delivery = &d
	}
	if ctx.PromptID != nil {
		prompt := *ctx.PromptID
		l.PromptID = &prompt
	}
	if ctx.AgentID != "" {
		agent := ctx.AgentID
		l.AgentID = &agent
	}
	if p.state == LineStateDelivered {
		l.ModelText, l.UserText = p.model, p.user
		l.Mirror, l.MirroredToUser = MirrorSystemMessage, true
	}
	return l
}
