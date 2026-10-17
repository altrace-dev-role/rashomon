package supervise

import (
	"errors"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestSTR4e_OnlyOneWellFormedObjectPassesTheGate: on exit 0 the parent
// forwards nothing, or one JSON object of the event's shape: allowlisted
// keys, hookEventName equal to the event, permissionDecision only deny or
// ask. Anything else is refused whole, never trimmed to what would pass.
func TestSTR4e_OnlyOneWellFormedObjectPassesTheGate(t *testing.T) {
	for _, c := range []struct {
		name, event, out string
		shape            bool
	}{
		{"empty", "PostToolUse", ``, true},
		{"whitespace only", "PostToolUse", " \n", true},
		{"recap's line", "Stop", `{"systemMessage":"rashomon: 3 calls"}`, true},
		{"recap's line, trailing newline", "Stop", `{"systemMessage":"rashomon: 3 calls"}` + "\n", true},
		{"stop failure line", "StopFailure", `{"systemMessage":"x"}`, true},
		{"prompt catch-up line", "UserPromptSubmit", `{"systemMessage":"x"}`, true},
		{"post note", "PostToolUse", `{"systemMessage":"x"}`, true},
		{"session start context", "SessionStart", `{"hookSpecificOutput":{"additionalContext":"c","hookEventName":"SessionStart"}}`, true},
		{"deny", "PreToolUse", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"r"},"systemMessage":"m"}`, true},
		{"ask", "PreToolUse", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"r"}}`, true},

		{"allow is never forwarded", "PreToolUse", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"r"}}`, false},
		{"decision without a reason", "PreToolUse", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}`, false},
		{"PreToolUse message without a decision", "PreToolUse", `{"systemMessage":"m"}`, false},
		{"decision on another event", "PostToolUse", `{"hookSpecificOutput":{"hookEventName":"PostToolUse","permissionDecision":"deny","permissionDecisionReason":"r"}}`, false},
		{"context on a per-call event", "PostToolUse", `{"hookSpecificOutput":{"additionalContext":"c","hookEventName":"PostToolUse"}}`, false},
		{"anything on SessionEnd", "SessionEnd", `{"systemMessage":"x"}`, false},
		{"wrong hookEventName", "SessionStart", `{"hookSpecificOutput":{"additionalContext":"c","hookEventName":"UserPromptSubmit"}}`, false},
		{"empty hookSpecificOutput", "SessionStart", `{"hookSpecificOutput":{"hookEventName":"SessionStart"}}`, false},
		{"unknown top-level key", "Stop", `{"continue":false,"systemMessage":"x"}`, false},
		{"unknown nested key", "SessionStart", `{"hookSpecificOutput":{"additionalContext":"c","hookEventName":"SessionStart","updatedInput":{}}}`, false},
		{"case-variant key", "Stop", `{"SystemMessage":"x"}`, false},
		{"duplicate key", "Stop", `{"systemMessage":"x","systemMessage":"y"}`, false},
		{"two objects", "Stop", `{"systemMessage":"x"}{"systemMessage":"y"}`, false},
		{"not an object", "Stop", `"x"`, false},
		{"not JSON", "Stop", `rashomon: hello`, false},
		{"empty message", "Stop", `{"systemMessage":""}`, false},
		{"null message", "Stop", `{"systemMessage":null}`, false},
		{"invalid UTF-8", "Stop", "{\"systemMessage\":\"\xff\"}", false},
		{"unknown event", "Elicitation", `{"systemMessage":"x"}`, false},
		{"no event", "", `{"systemMessage":"x"}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := shapeOK(c.event, []byte(c.out)); got != c.shape {
				t.Errorf("shapeOK(%s, %s) = %v, want %v", c.event, c.out, got, c.shape)
			}
		})
	}
}

// TestSTR4f_TheSteerSeamRefusesWhatSteerAloneMayPrint: until the steering
// core's checks are repeated here, anything only steering would print -- a
// decision, model-visible context -- is refused even when its shape passes;
// a plain systemMessage, the one thing printed today, passes.
func TestSTR4f_TheSteerSeamRefusesWhatSteerAloneMayPrint(t *testing.T) {
	for _, c := range []struct {
		event, out string
		want       bool
	}{
		{"Stop", `{"systemMessage":"rashomon: 3 calls"}`, true},
		{"Stop", ``, true},
		{"PreToolUse", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"r"}}`, false},
		{"SessionStart", `{"hookSpecificOutput":{"additionalContext":"c","hookEventName":"SessionStart"}}`, false},
	} {
		if got := Forwardable(c.event, []byte(c.out)); got != c.want {
			t.Errorf("Forwardable(%s, %s) = %v, want %v", c.event, c.out, got, c.want)
		}
	}
}

// TestSTR4g_TheGateAcceptsWhatSteerMarshals: the gate's table is its own --
// the parent does not link the steering core -- so it is held to the core's
// renderer here: every output steer.Output.Marshal produces for an event the
// gate knows passes the gate's shape check, and every event an entry is
// installed for has a row.
func TestSTR4g_TheGateAcceptsWhatSteerMarshals(t *testing.T) {
	outputs := []steer.Output{
		{SystemMessage: "m"},
		{AdditionalContext: "c"},
		{AdditionalContext: "c", SystemMessage: "m"},
		{Decision: &steer.PreDecision{Reason: "r"}},
		{Decision: &steer.PreDecision{Reason: "r", Ask: true}, SystemMessage: "m"},
		{StopContinue: "s", SystemMessage: "m"},
	}
	served := 0
	for _, r := range rows {
		for _, o := range outputs {
			b, err := o.Marshal(r.event)
			if err != nil {
				if errors.Is(err, steer.ErrUnknownEvent) {
					break
				}
				continue
			}
			served++
			if !shapeOK(r.event, b) {
				t.Errorf("steer marshals %s for %s and the gate refuses its shape", b, r.event)
			}
		}
	}
	if served == 0 {
		t.Fatal("no output was marshalled: the cross-check checked nothing")
	}
	for _, ev := range install.Events {
		if rowFor(ev) == nil {
			t.Errorf("installed event %s has no row in the gate's table", ev)
		}
	}
}
