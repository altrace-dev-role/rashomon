package steer

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestOutput_Marshal_EmptyReturnsNil(t *testing.T) {
	b, err := Output{}.Marshal(string(EventPreToolUse))
	if err != nil || b != nil {
		t.Fatalf("Marshal(empty) = (%v, %v), want (nil, nil)", b, err)
	}
}

func TestOutput_Marshal_UnknownEvent(t *testing.T) {
	_, err := Output{SystemMessage: "hi"}.Marshal("NotARealEvent")
	if !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("Marshal(unknown event) err = %v, want ErrUnknownEvent", err)
	}
}

func TestOutput_Marshal_DecisionWrongEvent(t *testing.T) {
	o := Output{Decision: &PreDecision{Ask: true, Reason: "because"}}
	_, err := o.Marshal(string(EventPostToolUse))
	if !errors.Is(err, ErrDecisionWrongEvent) {
		t.Fatalf("Marshal(Decision on PostToolUse) err = %v, want ErrDecisionWrongEvent", err)
	}
}

func TestOutput_Marshal_DecisionDenyOnPreToolUse(t *testing.T) {
	o := Output{Decision: &PreDecision{Ask: false, Reason: "denied: frozen path"}}
	b, err := o.Marshal(string(EventPreToolUse))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	hso, ok := got["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("hookSpecificOutput missing or wrong type: %v", got)
	}
	if hso["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want deny", hso["permissionDecision"])
	}
	if hso["permissionDecisionReason"] != "denied: frozen path" {
		t.Errorf("permissionDecisionReason = %v, want %q", hso["permissionDecisionReason"], "denied: frozen path")
	}
	if hso["hookEventName"] != string(EventPreToolUse) {
		t.Errorf("hookEventName = %v, want %q", hso["hookEventName"], EventPreToolUse)
	}
	if _, has := got["systemMessage"]; has {
		t.Errorf("systemMessage present with no SystemMessage set: %v", got)
	}
}

func TestOutput_Marshal_DecisionAskOnPreToolUse(t *testing.T) {
	o := Output{Decision: &PreDecision{Ask: true, Reason: "needs a human"}}
	b, err := o.Marshal(string(EventPreToolUse))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	hso := got["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "ask" {
		t.Errorf("permissionDecision = %v, want ask", hso["permissionDecision"])
	}
}

func TestOutput_Marshal_StopContinueWrongEvent(t *testing.T) {
	o := Output{StopContinue: "keep going"}
	_, err := o.Marshal(string(EventSessionStart))
	if !errors.Is(err, ErrStopContinueWrongEvent) {
		t.Fatalf("Marshal(StopContinue on SessionStart) err = %v, want ErrStopContinueWrongEvent", err)
	}
}

// TestOutput_Marshal_StopContinueIsAdditionalContextOnStop pins the Stop
// continuation's wire form (tech 4.2.3): additionalContext under
// hookEventName "Stop", and never a top-level decision, continue or a key
// of this package's own.
func TestOutput_Marshal_StopContinueIsAdditionalContextOnStop(t *testing.T) {
	o := Output{SystemMessage: "sys", StopContinue: "one more turn"}
	b, err := o.Marshal(string(EventStop))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, k := range []string{"decision", "continue", "reason", "stopReason", "stopContinue"} {
		if _, ok := got[k]; ok {
			t.Errorf("top-level %q emitted for a Stop continuation", k)
		}
	}
	hso, ok := got["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("no hookSpecificOutput in %s", b)
	}
	if hso["hookEventName"] != "Stop" || hso["additionalContext"] != "one more turn" {
		t.Errorf("hookSpecificOutput = %v, want hookEventName Stop and additionalContext %q", hso, "one more turn")
	}
	if _, ok := hso["stopContinue"]; ok {
		t.Errorf("hookSpecificOutput.stopContinue emitted: that key is nobody's")
	}
	for _, o := range []Output{
		{AdditionalContext: "a", StopContinue: "b"},
		{AdditionalContext: "a", StopContinue: "a"},
	} {
		if b, err := o.Marshal(string(EventStop)); !errors.Is(err, ErrAdditionalContextWrongEvent) || b != nil {
			t.Errorf("%+v on Stop = (%s, %v), want ErrAdditionalContextWrongEvent: Stop's additionalContext comes only from StopContinue", o, b, err)
		}
	}
}

// stdoutRow is what one event may print (archive 40.3.2's "Output STEER may
// emit" column; tech 4.2.3's per-event keys; G6: no PreToolUse output
// without a project stop). Written out here, not read from the code's own
// table, so the two are checked against each other.
type stdoutRow struct {
	context      bool // the AdditionalContext field
	decision     bool // permissionDecision with its reason, required for any output
	stopContinue bool // additionalContext built from StopContinue
}

var wantStdout = map[Event]stdoutRow{
	"PreToolUse":         {decision: true},
	"PostToolUse":        {},
	"PostToolUseFailure": {},
	"PostToolBatch":      {context: true},
	"UserPromptSubmit":   {context: true},
	"SessionStart":       {context: true},
	"SubagentStart":      {context: true},
	"Stop":               {stopContinue: true},
	"PostModelSwitch":    {},
}

// TestSTR3_OutputKeysAreAnAllowlist: Marshal prints a key only on an event
// whose row allows it, refuses the Output otherwise, and never prints a key
// outside the allowlist.
func TestSTR3_OutputKeysAreAnAllowlist(t *testing.T) {
	if len(wantStdout) != len(AllEvents()) {
		t.Fatalf("the table covers %d events, AllEvents() has %d", len(wantStdout), len(AllEvents()))
	}
	for _, ev := range AllEvents() {
		row, ok := wantStdout[ev]
		if !ok {
			t.Fatalf("no expected row for %q", ev)
		}
		e := string(ev)
		check := func(o Output, allowed bool, refusal error) {
			t.Helper()
			b, err := o.Marshal(e)
			if !allowed {
				if !errors.Is(err, refusal) || b != nil {
					t.Errorf("%s %+v = (%s, %v), want (nil, %v)", e, o, b, err, refusal)
				}
				return
			}
			if err != nil {
				t.Errorf("%s %+v: %v", e, o, err)
				return
			}
			assertAllowlisted(t, e, row, b)
		}
		check(Output{AdditionalContext: "ctx"}, row.context, ErrAdditionalContextWrongEvent)
		check(Output{SystemMessage: "sys", AdditionalContext: "ctx"}, row.context, ErrAdditionalContextWrongEvent)
		check(Output{SystemMessage: "sys"}, !row.decision, ErrDecisionRequired)
		check(Output{Decision: &PreDecision{Reason: "r"}}, row.decision, ErrDecisionWrongEvent)
		check(Output{SystemMessage: "mirror", Decision: &PreDecision{Ask: true, Reason: "r"}}, row.decision, ErrDecisionWrongEvent)
		check(Output{StopContinue: "c"}, row.stopContinue, ErrStopContinueWrongEvent)
		if row.decision {
			check(Output{AdditionalContext: "ctx", Decision: &PreDecision{Reason: "r"}}, false, ErrAdditionalContextWrongEvent)
		}
	}
}

// assertAllowlisted fails if b carries a key its event's row does not allow.
func assertAllowlisted(t *testing.T, event string, row stdoutRow, b []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("%s: %s is not a JSON object: %v", event, b, err)
	}
	for k := range top {
		if k != "systemMessage" && k != "hookSpecificOutput" {
			t.Errorf("%s: top-level key %q is outside the allowlist", event, k)
		}
	}
	raw, ok := top["hookSpecificOutput"]
	if !ok {
		return
	}
	var hso map[string]any
	if err := json.Unmarshal(raw, &hso); err != nil {
		t.Fatalf("%s: hookSpecificOutput %s: %v", event, raw, err)
	}
	allowed := map[string]bool{"hookEventName": true}
	if row.context || row.stopContinue {
		allowed["additionalContext"] = true
	}
	if row.decision {
		allowed["permissionDecision"] = true
		allowed["permissionDecisionReason"] = true
	}
	for k := range hso {
		if !allowed[k] {
			t.Errorf("%s: hookSpecificOutput.%s is outside the allowlist", event, k)
		}
	}
	if hso["hookEventName"] != event {
		t.Errorf("%s: hookEventName = %v", event, hso["hookEventName"])
	}
	if d, ok := hso["permissionDecision"]; ok && d != "deny" && d != "ask" {
		t.Errorf("%s: permissionDecision = %v, want deny or ask", event, d)
	}
}

// TestSTR9b_MarshalNeverRewritesBytes: encoding/json replaces invalid UTF-8
// with U+FFFD, so a mirror would print other bytes than the ones logged;
// Marshal refuses such a string instead.
func TestSTR9b_MarshalNeverRewritesBytes(t *testing.T) {
	bad := "rashomon record: \xff"
	for _, c := range []struct {
		event string
		o     Output
	}{
		{"SessionStart", Output{SystemMessage: bad}},
		{"SessionStart", Output{AdditionalContext: bad}},
		{"Stop", Output{StopContinue: bad}},
		{"PreToolUse", Output{Decision: &PreDecision{Reason: bad}}},
	} {
		if b, err := c.o.Marshal(c.event); !errors.Is(err, ErrInvalidUTF8) || b != nil {
			t.Errorf("%s %+v = (%q, %v), want ErrInvalidUTF8", c.event, c.o, b, err)
		}
	}
	if b, err := (Output{SystemMessage: "※ fine"}).Marshal("Stop"); err != nil || !strings.Contains(string(b), "※ fine") {
		t.Errorf("valid UTF-8 = (%s, %v), want it printed unchanged", b, err)
	}
}

func TestOutput_Marshal_SystemMessageOnlyNoHookSpecificOutput(t *testing.T) {
	o := Output{SystemMessage: "just a note"}
	b, err := o.Marshal(string(EventSessionStart))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, has := got["hookSpecificOutput"]; has {
		t.Errorf("hookSpecificOutput present with nothing hook-specific to say: %v", got)
	}
	if got["systemMessage"] != "just a note" {
		t.Errorf("systemMessage = %v, want %q", got["systemMessage"], "just a note")
	}
}
