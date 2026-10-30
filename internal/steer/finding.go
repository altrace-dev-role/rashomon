package steer

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// PatternID is pattern.ID under the name Technical spec 4.2.3's Finding
// listing uses. It is a type alias, not a new type, so a steer.PatternID
// and a pattern.ID are the same value and every pattern.ID constant
// (pattern.SubtaskGate, and so on) is already a valid Finding.Pattern
// without a conversion.
type PatternID = pattern.ID

// Kind is a Finding's sub-category under its Pattern -- one of the
// strings pattern.Kinds(id) lists for a pattern that has a closed
// sub-kind set, the caller-defined "<class>" name for declared-fact /
// learned-fact, or empty for a pattern with no sub-kind taxonomy at all.
// It is a distinct type from pattern.Kinds' own []string return so that
// Finding's field carries single-value semantics (Technical spec 4.2.3's
// Finding listing: "Kind Kind"), while pattern.Kinds keeps enumerating a
// whole vocabulary as plain strings.
type Kind string

// PreDecision is a PreToolUse permission decision: deny outright when Ask
// is false, or ask the human when Ask is true. Technical spec 4.2.3's
// Finding listing spells it "{Ask bool; Reason string}"; Output.Decision
// carries one.
type PreDecision struct {
	Ask    bool
	Reason string
}

// Finding is one pattern's claim about the current prompt, before Route
// (P-30) decides which findings win, which are suppressed, and what
// Output they become. Tech 4.2.3: "type Finding struct { // carries no free
// text" -- Message is the only field that holds text, and it holds a
// render.Message, which only render.New builds, so a Finding cannot carry
// text that skipped the renderer's checks.
type Finding struct {
	// Pattern is which of the eighteen frozen patterns produced this
	// Finding.
	Pattern PatternID
	// Kind is Pattern's sub-category, or empty if Pattern has none.
	Kind Kind
	// Key dedupes Finding instances within one prompt: two findings with
	// the same Pattern, Kind and Key are the same claim seen twice. It is
	// the card's unit of limit (archive 40.3.4: "fact id, gate family
	// digest, task id"), so the budget's one per key and two deny/ask per
	// key (tech 4.2.5) count per task, per family or per rule and file as
	// the card says.
	Key string
	// Priority is this Finding's step 8 ordering value (Technical spec
	// 4.2.4), normally pattern.Priority(Pattern)'s result at construction
	// time.
	Priority int
	// MaxLevel is the strongest Level this Finding may reach.
	MaxLevel Level
	// StopForm is what MaxLevel == LevelStop should look like in Output,
	// if this Finding is the one Route delivers at stop.
	StopForm StopForm
	// Message is Pattern's rendered claim (tech 4.2.3: "render.Message //
	// closed struct"), built by render.New; the zero Message carries no
	// text and is never delivered.
	Message render.Message
	// Evidence is the record seqs this Finding cites.
	Evidence []int64
	// AfterSeq is the last record seq the decision saw (archive 40.3.4):
	// the point the finding was evaluated at, never an older fact's seq,
	// so a record below it that arrives later is "recorded late" (tech
	// 4.2.13) and every evidence seq is at or below it.
	AfterSeq int64
	// AgentID is the subagent that produced this Finding, or empty for
	// the main agent.
	AgentID string
}

// ErrFinding is the error of every check Finding.Check fails; the message
// names the rule, never a value beyond the pattern and kind.
var ErrFinding = errors.New("steer: finding outside the contract")

// Check reports why f may not reach the router, or nil. Route calls it on
// every finding before routing (admit; TestDEL22f pins Route's body), so a
// producer cannot skip it. It holds the contract a producer can get wrong
// without a type error: Kind is one of Pattern's frozen kinds (pattern.KindValid; tech 72, "only frozen kinds cross");
// claim-vs-record/masked is never model-visible (ED-3: it stays in the
// user's Stop line, so it caps at flag with no stop form); MaxLevel is a
// level and StopForm a stop form, and a stop form other than none only at
// LevelStop (tech 4.2.4 step 5: stop needs a stop form); Evidence ascends
// strictly and no evidence seq is after AfterSeq, the last record seq the
// decision saw; and a rendered Message belongs to the Finding's own
// pattern, so a briefing's multi-line text cannot ride another pattern's
// single-line channel (output rule G4, tech 7.2.5, is the backstop).
func (f Finding) Check() error {
	switch {
	case !pattern.KindValid(f.Pattern, string(f.Kind)):
		return fmt.Errorf("%w: %q is not a frozen kind of %q", ErrFinding, f.Kind, f.Pattern)
	case userOnly(f) && (f.MaxLevel > LevelFlag || f.StopForm != StopFormNone):
		return fmt.Errorf("%w: %s/%s is user-only, never model-visible", ErrFinding, f.Pattern, f.Kind)
	case f.MaxLevel > LevelStop || !slices.Contains(AllStopForms(), f.StopForm):
		return fmt.Errorf("%w: level %d or stop form %q is outside its vocabulary", ErrFinding, f.MaxLevel, f.StopForm)
	case f.StopForm != StopFormNone && f.MaxLevel != LevelStop:
		return fmt.Errorf("%w: stop form %q below the stop level", ErrFinding, f.StopForm)
	case f.Message.Text() != "" && f.Message.Pattern() != f.Pattern:
		return fmt.Errorf("%w: a %q message on a %q finding", ErrFinding, f.Message.Pattern(), f.Pattern)
	}
	if p, ok := pattern.Priority(f.Pattern); !ok || f.Priority != p {
		return fmt.Errorf("%w: %s does not carry its own-channel or table priority", ErrFinding, f.Pattern)
	}
	return f.checkSeqs(1)
}

// CheckOn is Check for a finding routed on event, which admit takes from the
// hook's Context, never from the finding. UserPromptSubmit is the one event
// with no call before it: at a session's first prompt the run holds no record
// and the last seq the decision saw is 0, so there a finding may stand at
// after_seq 0, citing nothing. Every other event keeps Check's floor of 1.
func (f Finding) CheckOn(event Event) error {
	if event != EventUserPromptSubmit || f.AfterSeq != 0 {
		return f.Check()
	}
	if err := f.Check(); err == nil || !errors.Is(err, errAfterSeq) {
		return err
	}
	return f.checkSeqs(0)
}

// errAfterSeq is Check's after_seq floor alone, so CheckOn can lower that
// floor and keep every other rule.
var errAfterSeq = fmt.Errorf("%w: after_seq below its floor", ErrFinding)

// checkSeqs holds after_seq at floor or more, and every evidence seq at 1 or
// later, ascending, and at or before after_seq.
func (f Finding) checkSeqs(floor int64) error {
	if f.AfterSeq < floor {
		return errAfterSeq
	}
	for i, seq := range f.Evidence {
		if seq < 1 || seq > f.AfterSeq || i > 0 && seq <= f.Evidence[i-1] {
			return fmt.Errorf("%w: evidence seqs must ascend, start at 1 or later, and end at or before after_seq", ErrFinding)
		}
	}
	return nil
}

// userOnly reports a finding whose card keeps it out of the model's
// context at every level: claim-vs-record/masked (ED-3, the user's Stop
// line only), and the user-flag patterns integrity ("user flag only"; tech
// 4.2.4 step 5: "`integrity` set to `nudge` stays a flag"), sequence-risk
// (flag-only, Wave B) and model-switch ("user flag, fixed"; tech 4.2.13).
func userOnly(f Finding) bool {
	switch f.Pattern {
	case pattern.Integrity, pattern.SequenceRisk, pattern.ModelSwitch:
		return true
	}
	return f.Pattern == pattern.ClaimVsRecord && f.Kind == "masked"
}

// Output is the only value Route (tech 4.2.4) marshals to stdout (tech
// 4.2.3: "the only value marshalled to stdout"). A zero Output carries
// nothing to say.
type Output struct {
	// SystemMessage becomes the top-level "systemMessage" key, when
	// non-empty. On PreToolUse it may only accompany a Decision.
	SystemMessage string
	// AdditionalContext becomes hookSpecificOutput.additionalContext, when
	// non-empty, on PostToolBatch, SessionStart, SubagentStart and
	// UserPromptSubmit only; on any other event it is a Marshal error.
	AdditionalContext string
	// Decision is PreToolUse-only: non-nil on any other event is a
	// Marshal error.
	Decision *PreDecision
	// StopContinue is Stop-only, project-stop use: non-empty on any other
	// event is a Marshal error.
	StopContinue string
}

// ErrUnknownEvent is returned by Marshal for an event outside its table.
var ErrUnknownEvent = errors.New("steer: unknown hook event")

// ErrDecisionWrongEvent is returned by Marshal when Output.Decision is
// set on any event but PreToolUse (tech 4.2.3).
var ErrDecisionWrongEvent = errors.New("steer: Decision is PreToolUse-only")

// ErrStopContinueWrongEvent is returned by Marshal when
// Output.StopContinue is set on any event but Stop (tech 4.2.3).
var ErrStopContinueWrongEvent = errors.New("steer: StopContinue is Stop-only")

// ErrAdditionalContextWrongEvent is returned by Marshal when
// Output.AdditionalContext is set on an event whose row does not allow it,
// Stop included: Stop's additionalContext comes only from StopContinue.
var ErrAdditionalContextWrongEvent = errors.New("steer: AdditionalContext on an event that does not allow it")

// ErrDecisionRequired is returned by Marshal for PreToolUse output that
// carries no Decision (tech 4.2.1: "nothing or one deny/ask object"; G6:
// never "PreToolUse output without a project `stop`").
var ErrDecisionRequired = errors.New("steer: PreToolUse output without a decision")

// ErrInvalidUTF8 is returned by Marshal when a string field is not valid
// UTF-8, which encoding/json would otherwise print as U+FFFD.
var ErrInvalidUTF8 = errors.New("steer: output string is not valid UTF-8")

// ErrUnverifiedChannel is returned by MarshalFor, and so by Marshal, for
// AdditionalContext or a Decision on a row whose channel no live probe has
// verified for the Client (tech spec :2019; G3). It is a row the router
// ships in shadow with suppressed_by unverified_channel, not a malformed
// Output, so it stays distinct from the wrong-event errors.
var ErrUnverifiedChannel = errors.New("steer: model-visible output on a channel not verified for this client")

// eventKeys is one event's row of Marshal's key table: which optional keys
// the event may print. systemMessage is allowed on every event (on
// PreToolUse only beside a decision); hookEventName goes with any
// hookSpecificOutput key.
type eventKeys struct {
	// context: the AdditionalContext field may be set.
	context bool
	// decision: the output is one deny or ask object, so Decision may be
	// set and any output without one is refused.
	decision bool
	// stopContinue: StopContinue may be set; it is the event's only source
	// of additionalContext.
	stopContinue bool
	// exempt: the row's model-visible field needs no probed version (tech
	// spec :2019: UserPromptSubmit's mirror is the shipped catch-up). Every
	// other context or decision row is gated by mirrorVerifiedOn.
	exempt bool
}

// outputKeys is Marshal's per-event table (tech 4.2.3: "emits only
// per-event keys"; archive 40.3.2's "Output STEER may emit" column). The
// per-call events print AdditionalContext only where their channel is
// verified (tech spec :2019, which supersedes archive 40.3.5's "never"
// there): the row allows the key, and mirrorVerifiedOn decides whether it
// prints, so a per-call event never prints it ungated. PostModelSwitch
// prints a systemMessage only. An event absent from it is unknown.
func outputKeys() map[string]eventKeys {
	outputKeysOnce.Do(func() {
		outputKeysOnce.v = map[string]eventKeys{
			string(EventPreToolUse):         {decision: true},
			string(EventPostToolUse):        {context: true},
			string(EventPostToolUseFailure): {context: true},
			string(EventPostToolBatch):      {context: true},
			string(EventUserPromptSubmit):   {context: true, exempt: true},
			string(EventSessionStart):       {context: true},
			string(EventSubagentStart):      {context: true},
			string(EventStop):               {stopContinue: true},
			string(EventPostModelSwitch):    {},
		}
	})
	return outputKeysOnce.v
}

// outputKeysOnce holds what outputKeys returns, built on its first call and
// never at init (RES-2).
var outputKeysOnce struct {
	sync.Once
	v map[string]eventKeys
}

// Marshal renders Output for event, following tech 4.2.3's per-event key
// allowlist and nothing beyond it: the top-level "systemMessage" key, and
// hookSpecificOutput's "hookEventName", "additionalContext" and
// "permissionDecision" (with "permissionDecisionReason"), each only on the
// events outputKeys allows it. An Output with a field its event does not
// allow is refused whole, never printed with that field dropped. A Stop
// continuation is additionalContext on Stop (4.2.3; G6's reason for
// forbidding a top-level decision): Claude Code reads it as non-error
// feedback and continues under the same stop_hook_active protections, so
// StopContinue travels under that key and never as `decision` or
// `continue`; the AdditionalContext field is refused on Stop, because a
// nudge there would continue the turn without a project stop setting.
//
// permissionDecision carries only "deny" or "ask" (tech 4.2.3:
// "(`deny`/`ask`)") -- PreDecision.Ask selects between them; there is no
// "allow" value because STEER only ever narrows or holds a call, never
// grants one.
//
// An empty Output -- all four fields zero -- marshals to (nil, nil):
// there is nothing to say, so nothing is written, rather than an empty
// JSON object a caller would still have to special-case. Likewise,
// hookSpecificOutput itself is omitted (not emitted empty) when none of
// AdditionalContext, Decision or StopContinue is set, even if
// SystemMessage is. A string that is not valid UTF-8 is refused.
//
// Marshal is MarshalFor under the zero Client, an unknown version: it
// refuses AdditionalContext and a Decision on every gated row with
// ErrUnverifiedChannel, so no caller of it reaches a channel no probe
// verified. It does not apply output rule G2's 2,048-byte cap or G3's
// mirror-text check: those are the output gate's (tech 7.2.5).
func (o Output) Marshal(event string) ([]byte, error) {
	return o.MarshalFor(event, Client{})
}

// MarshalFor is Marshal for the client c: AdditionalContext on a context
// row, or a Decision on a decision row, prints only where the row is exempt
// or mirrorVerifiedOn lists c's version for it with the mirror verified and
// on the main agent; otherwise the Output is refused whole with
// ErrUnverifiedChannel, never printed with the field dropped. A
// systemMessage is never gated. The refusal comes after every check of the
// Output's own shape, so a malformed Output reports its defect first.
func (o Output) MarshalFor(event string, c Client) ([]byte, error) {
	return o.marshalFor(event, c, mirrorVerifiedOn())
}

// marshalFor is MarshalFor under table, the seam that lets a test reach a
// gated row's printing path.
func (o Output) marshalFor(event string, c Client, table map[string][]string) ([]byte, error) {
	keys, ok := outputKeys()[event]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEvent, event)
	}
	if err := o.check(event, keys); err != nil {
		return nil, err
	}
	if o.SystemMessage == "" && o.AdditionalContext == "" && o.Decision == nil && o.StopContinue == "" {
		return nil, nil
	}
	if keys.decision && o.Decision == nil {
		return nil, fmt.Errorf("%w: got %q", ErrDecisionRequired, event)
	}
	if err := o.checkUTF8(); err != nil {
		return nil, err
	}
	if o.AdditionalContext != "" && !verified(keys, event, c, table) {
		return nil, fmt.Errorf("%w: additionalContext on %q", ErrUnverifiedChannel, event)
	}
	if o.Decision != nil && !verified(keys, event, c, table) {
		return nil, fmt.Errorf("%w: a decision on %q", ErrUnverifiedChannel, event)
	}

	top := make(map[string]any, 2)
	if o.SystemMessage != "" {
		top["systemMessage"] = o.SystemMessage
	}

	hso := make(map[string]any, 4)
	if o.AdditionalContext != "" {
		hso["additionalContext"] = o.AdditionalContext
	}
	if o.StopContinue != "" {
		hso["additionalContext"] = o.StopContinue
	}
	if o.Decision != nil {
		if o.Decision.Ask {
			hso["permissionDecision"] = "ask"
		} else {
			hso["permissionDecision"] = "deny"
		}
		hso["permissionDecisionReason"] = o.Decision.Reason
	}
	if len(hso) > 0 {
		hso["hookEventName"] = event
		top["hookSpecificOutput"] = hso
	}

	buf, err := json.Marshal(top)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// check refuses a field event's row does not allow.
func (o Output) check(event string, keys eventKeys) error {
	if o.Decision != nil && !keys.decision {
		return fmt.Errorf("%w: got %q", ErrDecisionWrongEvent, event)
	}
	if o.StopContinue != "" && !keys.stopContinue {
		return fmt.Errorf("%w: got %q", ErrStopContinueWrongEvent, event)
	}
	if o.AdditionalContext != "" && !keys.context {
		return fmt.Errorf("%w: got %q", ErrAdditionalContextWrongEvent, event)
	}
	return nil
}

// checkUTF8 refuses any string field encoding/json would rewrite.
func (o Output) checkUTF8() error {
	reason := ""
	if o.Decision != nil {
		reason = o.Decision.Reason
	}
	for _, s := range []string{o.SystemMessage, o.AdditionalContext, o.StopContinue, reason} {
		if !utf8.ValidString(s) {
			return ErrInvalidUTF8
		}
	}
	return nil
}
