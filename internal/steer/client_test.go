package steer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// verifiedClient is the client P-TRU-1 passed on (PRD PF-ST4): Claude Code
// 2.1.280, read from its own environment, its in-band mirror verified on
// this surface, on the main agent.
var verifiedClient = Client{Version: "2.1.280", Source: VersionSourceEnv, MirrorInBand: true}

// tableVerifying lists v on every event, so marshalFor reaches each row's
// printing path while the compiled map keeps most rows empty.
func tableVerifying(v string) map[string][]string {
	table := make(map[string][]string, len(AllEvents()))
	for _, ev := range AllEvents() {
		table[string(ev)] = []string{v}
	}
	return table
}

// probedPairs is the compiled list written out here, not read from
// mirrorVerifiedOn, so the two are checked against each other: the only
// (event, version) pair a live P-TRU-1 run has passed (2026-10-23). A
// canary run that adds a version changes both.
var probedPairs = map[Event]string{EventPostToolUse: "2.1.280"}

// assertPrinted checks that b is o rendered for event: each model-visible
// field under its own key, and no hookSpecificOutput when o carries none.
func assertPrinted(t *testing.T, event string, o Output, b []byte) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("%s: %s is not a JSON object: %v", event, b, err)
	}
	if o.SystemMessage != "" && got["systemMessage"] != o.SystemMessage {
		t.Errorf("%s: systemMessage = %v, want %q", event, got["systemMessage"], o.SystemMessage)
	}
	hso, has := got["hookSpecificOutput"].(map[string]any)
	if o.AdditionalContext == "" && o.Decision == nil {
		if _, present := got["hookSpecificOutput"]; present {
			t.Errorf("%s: hookSpecificOutput present with nothing hook-specific to say: %s", event, b)
		}
		return
	}
	if !has {
		t.Fatalf("%s: no hookSpecificOutput in %s", event, b)
	}
	if o.AdditionalContext != "" && hso["additionalContext"] != o.AdditionalContext {
		t.Errorf("%s: additionalContext = %v, want %q", event, hso["additionalContext"], o.AdditionalContext)
	}
	if o.Decision != nil && (hso["permissionDecision"] != "deny" || hso["permissionDecisionReason"] != o.Decision.Reason) {
		t.Errorf("%s: permissionDecision %v, reason %v; want deny, %q", event, hso["permissionDecision"],
			hso["permissionDecisionReason"], o.Decision.Reason)
	}
}

// TestST4_PostToolUseContextAllowedOnlyWhenTheChannelIsVerified: a
// model-visible field prints only where a live probe passed on this exact
// client version, with the mirror verified and on the main agent (tech spec
// :2019, G3); everywhere else it is refused whole with ErrUnverifiedChannel,
// and the exempt row and a systemMessage alone are never gated.
func TestST4_PostToolUseContextAllowedOnlyWhenTheChannelIsVerified(t *testing.T) {
	with := func(f func(*Client)) Client {
		c := verifiedClient
		f(&c)
		return c
	}
	ctx := Output{SystemMessage: `※ rashomon told the agent: "run the gate"`, AdditionalContext: "run the gate"}
	decision := Output{Decision: &PreDecision{Reason: "denied: frozen path"}}
	for _, tc := range []struct {
		name  string
		event Event
		c     Client
		table map[string][]string // nil: the compiled map, through MarshalFor
		o     Output
		want  error // nil: printed
	}{
		{"PostToolUse on the probed version", EventPostToolUse, verifiedClient, nil, ctx, nil},
		{"a newer version", EventPostToolUse, with(func(c *Client) { c.Version = "2.1.281" }), nil, ctx, ErrUnverifiedChannel},
		{"a prefix of the probed version", EventPostToolUse, with(func(c *Client) { c.Version = "2.1.28" }), nil, ctx, ErrUnverifiedChannel},
		{"the probed version as a prefix", EventPostToolUse, with(func(c *Client) { c.Version = "2.1.2800" }), nil, ctx, ErrUnverifiedChannel},
		{"a subagent's call", EventPostToolUse, with(func(c *Client) { c.Subagent = true }), nil, ctx, ErrUnverifiedChannel},
		{"the mirror unverified on this surface", EventPostToolUse, with(func(c *Client) { c.MirrorInBand = false }), nil, ctx, ErrUnverifiedChannel},
		{"the zero Client", EventPostToolUse, Client{}, nil, ctx, ErrUnverifiedChannel},
		{"PostToolUseFailure, never probed", EventPostToolUseFailure, verifiedClient, nil, ctx, ErrUnverifiedChannel},
		{"SessionStart, never probed", EventSessionStart, verifiedClient, nil, ctx, ErrUnverifiedChannel},
		{"UserPromptSubmit is exempt", EventUserPromptSubmit, Client{}, nil, ctx, nil},
		{"Stop takes no AdditionalContext", EventStop, verifiedClient, nil, ctx, ErrAdditionalContextWrongEvent},
		{"a systemMessage alone is never gated", EventPostToolUse, Client{}, nil, Output{SystemMessage: "just a note"}, nil},
		{"a PreToolUse decision under the zero Client", EventPreToolUse, Client{}, nil, decision, ErrUnverifiedChannel},
		{"a PreToolUse decision on 2.1.280, never probed", EventPreToolUse, verifiedClient, nil, decision, ErrUnverifiedChannel},
		{"a PreToolUse decision where the table lists the version", EventPreToolUse, verifiedClient, tableVerifying("2.1.280"), decision, nil},
		{"a table without the row reads gated", EventPostToolUse, verifiedClient, map[string][]string{}, ctx, ErrUnverifiedChannel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := string(tc.event)
			var b []byte
			var err error
			if tc.table == nil {
				b, err = tc.o.MarshalFor(e, tc.c)
			} else {
				b, err = tc.o.marshalFor(e, tc.c, tc.table)
			}
			if tc.want != nil {
				if !errors.Is(err, tc.want) || b != nil {
					t.Fatalf("%s %+v = (%s, %v), want (nil, %v)", e, tc.c, b, err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s %+v: %v, want it printed", e, tc.c, err)
			}
			assertPrinted(t, e, tc.o, b)
		})
	}
}

// TestTRU12_UnknownClientVersionShipsInShadow: the zero Client, which is
// what an unreadable version gives, opens no gated row; across clients,
// ContextAllowed is true, and MarshalFor prints a model-visible field, only
// for the probed (event, version) pairs with the mirror verified and no
// subagent, and the predicate and the marshal never disagree.
func TestTRU12_UnknownClientVersionShipsInShadow(t *testing.T) {
	for _, ev := range AllEvents() {
		if got, want := ContextAllowed(string(ev), Client{}), ev == EventUserPromptSubmit; got != want {
			t.Errorf("ContextAllowed(%s, zero Client) = %v, want %v", ev, got, want)
		}
	}
	if ContextAllowed("NotARealEvent", verifiedClient) {
		t.Errorf("ContextAllowed on an unknown event = true")
	}
	clients := []Client{
		{},
		verifiedClient,
		{Version: "2.1.280", Source: VersionSourceEnv},
		{Version: "2.1.280", Source: VersionSourceEnv, MirrorInBand: true, Subagent: true},
		{Version: "2.1.281", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "2.1.257", Source: VersionSourceEnv, MirrorInBand: true},
	}
	ctx := Output{AdditionalContext: "ctx"}
	decision := Output{Decision: &PreDecision{Reason: "r"}}
	for _, ev := range AllEvents() {
		row := wantStdout[ev]
		e := string(ev)
		for _, c := range clients {
			v, probed := probedPairs[ev]
			open := ev == EventUserPromptSubmit || probed && c.Version == v && c.MirrorInBand && !c.Subagent
			if got := ContextAllowed(e, c); got != (row.context && open) {
				t.Errorf("ContextAllowed(%s, %+v) = %v, want %v", e, c, got, row.context && open)
			}
			for _, o := range []Output{ctx, decision} {
				if o.Decision != nil && !row.decision || o.AdditionalContext != "" && !row.context {
					continue
				}
				b, err := o.MarshalFor(e, c)
				switch {
				case open && err != nil:
					t.Errorf("%s %+v %+v: %v, want it printed", e, c, o, err)
				case !open && (!errors.Is(err, ErrUnverifiedChannel) || b != nil):
					t.Errorf("%s %+v %+v = (%s, %v), want ErrUnverifiedChannel", e, c, o, b, err)
				}
			}
		}
	}
}

// TestST4b_MarshalIsMarshalForWithTheZeroClient: Marshal is the gate under
// the zero Client, byte for byte and error for error, so no caller of
// Marshal reaches a gated row.
func TestST4b_MarshalIsMarshalForWithTheZeroClient(t *testing.T) {
	outputs := []Output{
		{},
		{SystemMessage: "sys"},
		{AdditionalContext: "ctx"},
		{SystemMessage: "sys", AdditionalContext: "ctx"},
		{Decision: &PreDecision{Reason: "r"}},
		{SystemMessage: "mirror", Decision: &PreDecision{Ask: true, Reason: "r"}},
		{StopContinue: "c"},
		{AdditionalContext: "rashomon record: \xff"},
	}
	for _, ev := range AllEvents() {
		e := string(ev)
		for _, o := range outputs {
			b1, err1 := o.Marshal(e)
			b2, err2 := o.MarshalFor(e, Client{})
			if !bytes.Equal(b1, b2) || fmt.Sprint(err1) != fmt.Sprint(err2) {
				t.Errorf("%s %+v: Marshal = (%s, %v), MarshalFor(zero) = (%s, %v)", e, o, b1, err1, b2, err2)
			}
		}
	}
}

// TestST4c_ClientVersionReadsOnlyTheHarnessForm: the version is read from
// the form Claude Code gives its hooks (AI_AGENT=claude-code_<version with
// dashes>_harness, read from the 2.1.280 binary and measured on a
// SessionStart hook); every other value is unknown, which is shadow.
func TestST4c_ClientVersionReadsOnlyTheHarnessForm(t *testing.T) {
	for _, tc := range []struct{ in, version, source string }{
		{"claude-code_2-1-280_harness", "2.1.280", VersionSourceEnv},
		{"claude-code_2-1_harness", "2.1", VersionSourceEnv},
		{"claude-code_1-2-3-4_harness", "1.2.3.4", VersionSourceEnv},
		{"claude-code_2-1-28000000_harness", "2.1.28000000", VersionSourceEnv}, // 32 bytes exactly
		{"", "", VersionSourceUnknown},
		{"claude-code_2-1-280_agent", "", VersionSourceUnknown},
		{"claude-code/2.1.280", "", VersionSourceUnknown},
		{"cursor_1-2-3_harness", "", VersionSourceUnknown},
		{"claude-code_2_harness", "", VersionSourceUnknown},
		{"claude-code_1-2-3-4-5_harness", "", VersionSourceUnknown},
		{"claude-code_2-1-28a_harness", "", VersionSourceUnknown},
		{"claude-code_2-1-280000000_harness", "", VersionSourceUnknown}, // 33 bytes
		{"claude-code_2--1_harness", "", VersionSourceUnknown},
		{"claude-code_2-1-_harness", "", VersionSourceUnknown},
		{"claude-code_-2-1_harness", "", VersionSourceUnknown},
		{"claude-code_2-1-280_harness ", "", VersionSourceUnknown},
	} {
		v, s := ClientVersion(tc.in)
		if v != tc.version || s != tc.source {
			t.Errorf("ClientVersion(%q) = (%q, %q), want (%q, %q)", tc.in, v, s, tc.version, tc.source)
		}
	}
	if n := len("claude-code_2-1-28000000_harness"); n != 32 {
		t.Fatalf("the boundary fixture is %d bytes, not 32", n)
	}
}
