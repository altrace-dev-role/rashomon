package supervise

import (
	"bytes"
	"encoding/json"
)

// row is one event's line in the output gate's table: which keys a child's
// output may carry on that event.
//
// It is the parent's own table, not the steering core's renderer, which the
// parent does not link: a test holds the two together. It has rows the
// renderer has not -- StopFailure, whose recap line is printed today, and
// SessionEnd, on which nothing is printed -- and is otherwise the same:
// systemMessage anywhere but SessionEnd, and on PreToolUse only beside a
// decision; model-visible additionalContext on the session and prompt
// events and Stop; a decision on PreToolUse alone.
type row struct {
	event    string
	system   bool
	context  bool
	decision bool
}

var rows = [...]row{
	{event: "PreToolUse", system: true, decision: true},
	{event: "PostToolUse", system: true},
	{event: "PostToolUseFailure", system: true},
	{event: "SessionStart", system: true, context: true},
	{event: "SessionEnd"},
	{event: "Stop", system: true, context: true},
	{event: "StopFailure", system: true},
	{event: "UserPromptSubmit", system: true, context: true},
}

// rowFor finds an event's row; there is none for an event outside the table.
func rowFor(event string) *row {
	for i := range rows {
		if rows[i].event == event {
			return &rows[i]
		}
	}
	return nil
}

// output is the whole of what a child may print, with its keys in the order
// encoding/json writes a map's: re-encoding a decoded output gives back the
// child's own bytes exactly when they held these keys and no others, once
// each, spelt exactly so.
type output struct {
	HookSpecificOutput *specific `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string    `json:"systemMessage,omitempty"`
}

type specific struct {
	AdditionalContext        string  `json:"additionalContext,omitempty"`
	HookEventName            string  `json:"hookEventName"`
	PermissionDecision       string  `json:"permissionDecision,omitempty"`
	PermissionDecisionReason *string `json:"permissionDecisionReason,omitempty"`
}

// Forwardable reports whether a child that exited 0 may have out forwarded
// for event: empty, or one JSON object of the event's shape that the steering
// checks also pass. Anything else is refused whole, never trimmed to what
// would pass.
func Forwardable(event string, out []byte) bool {
	return shapeOK(event, out) && recheckSteer(event, bytes.TrimSpace(out))
}

// shapeOK is the shape check: allowlisted keys for the event, hookEventName
// equal to it, permissionDecision deny or ask with a reason, and nothing
// that does not re-encode to exactly the bytes the child wrote. Decoding
// alone would not do: encoding/json matches keys without regard to case,
// keeps the last of a repeated key and replaces invalid UTF-8, each of which
// the byte comparison turns into a refusal.
func shapeOK(event string, out []byte) bool {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return true
	}
	r := rowFor(event)
	if r == nil {
		return false
	}
	var o output
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return false
	}
	again, err := json.Marshal(o)
	if err != nil || !bytes.Equal(again, out) {
		return false
	}
	if o.SystemMessage != "" && !r.system {
		return false
	}
	h := o.HookSpecificOutput
	if h == nil {
		// A PreToolUse message rides only beside a decision.
		return o.SystemMessage != "" && !r.decision
	}
	if h.HookEventName != event {
		return false
	}
	hasContext := h.AdditionalContext != ""
	hasDecision := h.PermissionDecision != "" || h.PermissionDecisionReason != nil
	switch {
	case !hasContext && !hasDecision:
		return false
	case hasContext && !r.context:
		return false
	case hasDecision && !r.decision:
		return false
	case r.decision && !hasDecision:
		return false
	}
	if hasDecision {
		if h.PermissionDecision != "deny" && h.PermissionDecision != "ask" {
			return false
		}
		if h.PermissionDecisionReason == nil {
			return false
		}
	}
	return true
}

// recheckSteer is the seam for the steering checks the parent repeats on
// what a child prints: a deny or ask only for a pattern the project set to
// stop, the mirror byte for byte, the final-string checks. None of them is
// built yet, so whatever only steering would print is refused here -- every
// decision and every piece of model-visible context -- and a plain
// systemMessage, the only output an entry has today, passes.
func recheckSteer(_ string, out []byte) bool {
	if len(out) == 0 {
		return true
	}
	var o output
	if err := json.Unmarshal(out, &o); err != nil {
		return false
	}
	return o.HookSpecificOutput == nil
}
