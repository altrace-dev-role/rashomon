package steer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// verifiedClient is the client P-TRU-1 passed on (PRD PF-ST4): Claude Code
// 2.1.280, read from its own environment, its in-band mirror verified on
// this surface, on the main agent.
var verifiedClient = Client{Version: "2.1.280", Source: VersionSourceEnv, MirrorInBand: true}

// tableVerifying gives every event the floor v, so marshalFor reaches each
// row's printing path while the compiled table keeps most rows closed.
func tableVerifying(v string) map[string]channelBasis {
	table := make(map[string]channelBasis, len(AllEvents()))
	for _, ev := range AllEvents() {
		table[string(ev)] = channelBasis{floor: v}
	}
	return table
}

// probedFloors is the compiled table written out here, not read from
// mirrorVerifiedOn, so the two are checked against each other: the only
// floor a live P-TRU-1 run has set (2026-10-23, on 2.1.280), and no
// exclusion. A canary run that sets a floor or excludes a version changes
// both.
var probedFloors = map[Event]channelBasis{EventPostToolUse: {floor: "2.1.280"}}

// openFor is whether probedFloors verifies c on ev, written without
// compareVersions so the test does not share the code it checks: the floor
// and exclusions here are all 2.1.x, so a 2.1.x client compares on its last
// component, and the other clients the tests use are listed by hand.
func openFor(ev Event, c Client) bool {
	b, probed := probedFloors[ev]
	if !probed || !c.MirrorInBand || c.Subagent {
		return false
	}
	switch c.Version {
	case "2.2.0", "3.0", "2.1.2800", "2.1.0280":
		return true
	case "", "2.0.999", "2.1.x", "3":
		return false
	}
	last, ok := strings.CutPrefix(c.Version, "2.1.")
	n, err := strconv.Atoi(last)
	floor, _ := strconv.Atoi(strings.TrimPrefix(b.floor, "2.1."))
	return ok && err == nil && n >= floor && !slices.Contains(b.excluded, c.Version)
}

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
// model-visible field prints only where the client's version is at or above
// the floor a live probe set for the event, compared by component, and is
// not excluded, with the mirror verified and on the main agent (tech spec
// :2019, G3); everywhere else, a malformed version included, it is refused
// whole with ErrUnverifiedChannel, and the exempt row and a systemMessage
// alone are never gated.
func TestST4_PostToolUseContextAllowedOnlyWhenTheChannelIsVerified(t *testing.T) {
	with := func(f func(*Client)) Client {
		c := verifiedClient
		f(&c)
		return c
	}
	version := func(v string) Client { return with(func(c *Client) { c.Version = v }) }
	ctx := Output{SystemMessage: `※ rashomon told the agent: "run the gate"`, AdditionalContext: "run the gate"}
	decision := Output{Decision: &PreDecision{Reason: "denied: frozen path"}}
	for _, tc := range []struct {
		name  string
		event Event
		c     Client
		table map[string]channelBasis // nil: the compiled table, through MarshalFor
		o     Output
		want  error // nil: printed
	}{
		{"PostToolUse on the floor", EventPostToolUse, verifiedClient, nil, ctx, nil},
		{"a newer patch", EventPostToolUse, version("2.1.281"), nil, ctx, nil},
		{"the version Rishi ran, 2.1.314", EventPostToolUse, version("2.1.314"), nil, ctx, nil},
		{"a newer minor", EventPostToolUse, version("2.2.0"), nil, ctx, nil},
		{"a newer major, two components", EventPostToolUse, version("3.0"), nil, ctx, nil},
		{"the floor with a leading zero", EventPostToolUse, version("2.1.0280"), nil, ctx, nil},
		{"the floor with a fourth component", EventPostToolUse, version("2.1.280.0"), nil, ctx, nil},
		{"the floor as a prefix, numerically above it", EventPostToolUse, version("2.1.2800"), nil, ctx, nil},
		{"one below the floor", EventPostToolUse, version("2.1.279"), nil, ctx, ErrUnverifiedChannel},
		{"a prefix of the floor, numerically below it", EventPostToolUse, version("2.1.28"), nil, ctx, ErrUnverifiedChannel},
		{"an older minor with a larger patch", EventPostToolUse, version("2.0.999"), nil, ctx, ErrUnverifiedChannel},
		{"a malformed version", EventPostToolUse, version("2.1.x"), nil, ctx, ErrUnverifiedChannel},
		{"one component, above the floor if it were compared", EventPostToolUse, version("3"), nil, ctx, ErrUnverifiedChannel},
		{"a version with a suffix", EventPostToolUse, version("2.1.300-beta"), nil, ctx, ErrUnverifiedChannel},
		{"an empty version with the mirror verified", EventPostToolUse, version(""), nil, ctx, ErrUnverifiedChannel},
		{"an excluded version", EventPostToolUse, verifiedClient, map[string]channelBasis{string(EventPostToolUse): {floor: "2.1.280", excluded: []string{"2.1.280"}}}, ctx, ErrUnverifiedChannel},
		{"a subagent's call", EventPostToolUse, with(func(c *Client) { c.Subagent = true }), nil, ctx, ErrUnverifiedChannel},
		{"the mirror unverified on this surface", EventPostToolUse, with(func(c *Client) { c.MirrorInBand = false }), nil, ctx, ErrUnverifiedChannel},
		{"the zero Client", EventPostToolUse, Client{}, nil, ctx, ErrUnverifiedChannel},
		{"PostToolUseFailure, never probed", EventPostToolUseFailure, verifiedClient, nil, ctx, ErrUnverifiedChannel},
		{"SessionStart, never probed", EventSessionStart, verifiedClient, nil, ctx, ErrUnverifiedChannel},
		{"SessionStart on a far newer version", EventSessionStart, version("99.0.0"), nil, ctx, ErrUnverifiedChannel},
		{"UserPromptSubmit is exempt", EventUserPromptSubmit, Client{}, nil, ctx, nil},
		{"Stop takes no AdditionalContext", EventStop, verifiedClient, nil, ctx, ErrAdditionalContextWrongEvent},
		{"a systemMessage alone is never gated", EventPostToolUse, Client{}, nil, Output{SystemMessage: "just a note"}, nil},
		{"a PreToolUse decision under the zero Client", EventPreToolUse, Client{}, nil, decision, ErrUnverifiedChannel},
		{"a PreToolUse decision on 2.1.280, never probed", EventPreToolUse, verifiedClient, nil, decision, ErrUnverifiedChannel},
		{"a PreToolUse decision where the table lists the version", EventPreToolUse, verifiedClient, tableVerifying("2.1.280"), decision, nil},
		{"a table without the row reads gated", EventPostToolUse, verifiedClient, map[string]channelBasis{}, ctx, ErrUnverifiedChannel},
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
// on the events probedFloors gives a floor, for a version at or above it,
// with the mirror verified and no subagent, and the predicate and the
// marshal never disagree.
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
		{Version: "2.1.314", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "2.1.257", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "2.1.279", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "2.2.0", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "2.0.999", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "2.1.x", Source: VersionSourceEnv, MirrorInBand: true},
		{Version: "3", Source: VersionSourceEnv, MirrorInBand: true},
	}
	ctx := Output{AdditionalContext: "ctx"}
	decision := Output{Decision: &PreDecision{Reason: "r"}}
	for _, ev := range AllEvents() {
		row := wantStdout[ev]
		e := string(ev)
		for _, c := range clients {
			open := ev == EventUserPromptSubmit || openFor(ev, c)
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

// TestVG1a_VersionsCompareByComponent: compareVersions orders dotted versions
// component by component, as numbers: 2.1.314 is above 2.1.280, 2.1.2800
// above 2.1.280 although it starts with it, 2.1.28 below it, a leading zero
// changes nothing, and a missing component reads as 0. Components longer
// than any integer type compare without overflow. The order is
// antisymmetric on every pair.
func TestVG1a_VersionsCompareByComponent(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"2.1.280", "2.1.280", 0},
		{"2.1.314", "2.1.280", 1},
		{"2.1.281", "2.1.280", 1},
		{"2.1.279", "2.1.280", -1},
		{"2.1.2800", "2.1.280", 1},
		{"2.1.28", "2.1.280", -1},
		{"2.2.0", "2.1.999", 1},
		{"2.0.999", "2.1.0", -1},
		{"3.0", "2.1.280", 1},
		{"10.0.0", "9.99.99", 1},
		{"2.1.0280", "2.1.280", 0},
		{"02.01.280", "2.1.280", 0},
		{"2.1", "2.1.0", 0},
		{"2.1.280.0", "2.1.280", 0},
		{"2.1.280.1", "2.1.280", 1},
		{"2.1", "2.1.1", -1},
		{"0.0", "0.0.0.0", 0},
		{"2.1.99999999999999999999", "2.1.99999999999999999998", 1},
		{"2.1.000000000000000000001", "2.1.1", 0},
	} {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := compareVersions(tc.b, tc.a); got != -tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.b, tc.a, got, -tc.want)
		}
	}
	if n := testing.AllocsPerRun(100, func() { compareVersions("2.1.0314", "2.1.280") }); n != 0 {
		t.Errorf("compareVersions allocates %v times per call, want 0", n)
	}
}

// TestVG1b_FloorAndExclusionPerEvent: each event reads its own row. Under a
// table that gives PostToolUse the floor 2.1.280 with 2.1.300 excluded and
// SessionStart the floor 2.1.300, a version is verified on an event only at
// or above that event's floor and only when it equals none of that event's
// exclusions, by component; an event the table does not name is closed. Each
// refusal names its reason (shutBy), the reason the notice prints.
func TestVG1b_FloorAndExclusionPerEvent(t *testing.T) {
	table := map[string]channelBasis{
		string(EventPostToolUse):  {floor: "2.1.280", excluded: []string{"2.1.300"}},
		string(EventSessionStart): {floor: "2.1.300"},
	}
	at := func(v string) Client { return Client{Version: v, Source: VersionSourceEnv, MirrorInBand: true} }
	for _, tc := range []struct {
		event Event
		c     Client
		want  shutReason
	}{
		{EventPostToolUse, at("2.1.280"), channelOpen},
		{EventPostToolUse, at("2.1.299"), channelOpen},
		{EventPostToolUse, at("2.1.300"), shutExcluded},
		{EventPostToolUse, at("2.1.0300"), shutExcluded},
		{EventPostToolUse, at("2.1.300.0"), shutExcluded},
		{EventPostToolUse, at("2.1.301"), channelOpen},
		{EventPostToolUse, at("2.1.279"), shutBelowFloor},
		{EventSessionStart, at("2.1.299"), shutBelowFloor},
		{EventSessionStart, at("2.1.300"), channelOpen},
		{EventPostToolUseFailure, at("2.1.300"), shutEventClosed},
		{EventPostToolUse, at(""), shutUnknownVersion},
		{EventPostToolUse, at("2.1.x"), shutUnknownVersion},
		{EventPostToolUse, at("9"), shutUnknownVersion},
		{EventPostToolUse, Client{Version: "2.1.301"}, shutNoMirror},
		{EventPostToolUse, Client{Version: "2.1.301", MirrorInBand: true, Subagent: true}, shutSubagent},
		{EventUserPromptSubmit, Client{}, channelOpen},
	} {
		e := string(tc.event)
		keys := outputKeys()[e]
		if got := shutBy(keys, e, tc.c, table); got != tc.want {
			t.Errorf("shutBy(%s, %+v) = %d, want %d", e, tc.c, got, tc.want)
		}
		if got, want := allowed(e, tc.c, table), tc.want == channelOpen; got != want {
			t.Errorf("allowed(%s, %+v) = %v, want %v", e, tc.c, got, want)
		}
	}
}

// TestVG1c_TheCompiledTableIsTheProbedOne: mirrorVerifiedOn is probedFloors
// exactly on every gated row, so a floor or an exclusion moves only with this
// file; SessionStart, SubagentStart, PostToolBatch and PreToolUse stay closed
// with no exclusion, on any version; every floor and exclusion is a version
// ClientVersion could return, and every exclusion is at or above its floor,
// since one below it excludes nothing.
func TestVG1c_TheCompiledTableIsTheProbedOne(t *testing.T) {
	table := mirrorVerifiedOn()
	for _, ev := range AllEvents() {
		keys := outputKeys()[string(ev)]
		if keys.exempt || !keys.context && !keys.decision {
			continue
		}
		got, want := table[string(ev)], probedFloors[ev]
		if got.floor != want.floor || !reflect.DeepEqual(slices.Clip(got.excluded), slices.Clip(want.excluded)) && len(got.excluded)+len(want.excluded) > 0 {
			t.Errorf("%s: compiled %+v, probed %+v", ev, got, want)
		}
	}
	for _, ev := range []Event{EventSessionStart, EventSubagentStart, EventPostToolBatch, EventPreToolUse} {
		if b := table[string(ev)]; b.floor != "" || len(b.excluded) != 0 {
			t.Errorf("%s: %+v, want closed with no exclusion", ev, b)
		}
		if c := (Client{Version: "99.99.99", Source: VersionSourceEnv, MirrorInBand: true}); shutBy(outputKeys()[string(ev)], string(ev), c, table) != shutEventClosed {
			t.Errorf("%s opens on 99.99.99", ev)
		}
	}
	for ev, b := range table {
		if b.floor == "" && len(b.excluded) > 0 {
			t.Errorf("%s: exclusions %v on a closed row", ev, b.excluded)
		}
		if b.floor != "" && !digitRuns(b.floor, '.') {
			t.Errorf("%s: floor %q is not a dotted version", ev, b.floor)
		}
		for _, x := range b.excluded {
			if !digitRuns(x, '.') || compareVersions(x, b.floor) < 0 {
				t.Errorf("%s: exclusion %q is malformed or below the floor %q", ev, x, b.floor)
			}
		}
	}
}
