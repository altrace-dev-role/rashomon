package steer

import (
	"encoding/json"
	"errors"
	"fmt"
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
	// Pattern is which of the sixteen frozen patterns produced this
	// Finding.
	Pattern PatternID
	// Kind is Pattern's sub-category, or empty if Pattern has none.
	Kind Kind
	// Key dedupes Finding instances within one prompt: two findings with
	// the same Pattern, Kind and Key are the same claim seen twice.
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
	// AfterSeq is the record seq this Finding was raised after.
	AfterSeq int64
	// AgentID is the subagent that produced this Finding, or empty for
	// the main agent.
	AgentID string
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
}

// outputKeys is Marshal's per-event table (tech 4.2.3: "emits only
// per-event keys"; archive 40.3.2's "Output STEER may emit" column). The
// per-call events and PostModelSwitch print a systemMessage only (archive
// 40.3.5: "Per-call events (PostToolUse, PostToolUseFailure) never produce
// `AdditionalContext`"). An event absent from it is unknown.
var outputKeys = map[string]eventKeys{
	string(EventPreToolUse):         {decision: true},
	string(EventPostToolUse):        {},
	string(EventPostToolUseFailure): {},
	string(EventPostToolBatch):      {context: true},
	string(EventUserPromptSubmit):   {context: true},
	string(EventSessionStart):       {context: true},
	string(EventSubagentStart):      {context: true},
	string(EventStop):               {stopContinue: true},
	string(EventPostModelSwitch):    {},
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
// SystemMessage is. A string that is not valid UTF-8 is refused. Marshal
// does not apply output rule G2's 2,048-byte cap or G3's mirror check:
// those are the output gate's (tech 7.2.5).
func (o Output) Marshal(event string) ([]byte, error) {
	keys, ok := outputKeys[event]
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
