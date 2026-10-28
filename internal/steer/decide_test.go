package steer

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// zeroFinding is an admitted finding with the zero Message, which Check
// admits: the ordering is the pattern's priority, and only
// regression-location has a template row (QA lens 3).
func zeroFinding(p PatternID, kind Kind, afterSeq int64, key string) Finding {
	prio, _ := pattern.Priority(p)
	return Finding{Pattern: p, Kind: kind, Priority: prio, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: afterSeq, Key: key}
}

// TestST1_HighestPriorityWins: step 8's order (tech 4.2.4) on the pure rank
// function: priority descending, then the lower after_seq, then input order,
// a stable sort; step 8's "then steer id" is superseded, because ids are
// minted after the sort (card 2.2 R5).
func TestST1_HighestPriorityWins(t *testing.T) {
	in := []Finding{
		zeroFinding(pattern.RegressionLocation, "", 9, "r"),
		zeroFinding(pattern.SubtaskGate, "", 9, "s"),
		zeroFinding(pattern.ClaimVsRecord, "red", 9, "c"),
	}
	for _, f := range in {
		if err := f.Check(); err != nil {
			t.Fatalf("%s does not pass Check: %v", f.Pattern, err)
		}
	}
	got := rank(in)
	if got[0].Pattern != pattern.SubtaskGate || got[1].Pattern != pattern.ClaimVsRecord || got[2].Pattern != pattern.RegressionLocation {
		t.Errorf("rank = %s, %s, %s, want subtask-gate 90, claim-vs-record 85, regression-location 80", got[0].Pattern, got[1].Pattern, got[2].Pattern)
	}
	ties := rank([]Finding{
		zeroFinding(pattern.RegressionLocation, "", 7, "late"),
		zeroFinding(pattern.RegressionLocation, "", 3, "first-of-3"),
		zeroFinding(pattern.RegressionLocation, "", 3, "second-of-3"),
	})
	if ties[0].Key != "first-of-3" || ties[1].Key != "second-of-3" || ties[2].Key != "late" {
		t.Errorf("ties = %s, %s, %s, want the lower after_seq first, then input order", ties[0].Key, ties[1].Key, ties[2].Key)
	}
}

// TestST_NoteTextsRefuses tests the two text refusals directly (card 2.2
// R7, A8): neither is reachable through Route, because a Message comes only
// from render.New, which has validated it under 1,200 bytes, and its mirror
// is at most 1,229 bytes, under the 2,048-byte cap. A text over the cap is
// withheld (output_gate, G2); a text Validate refuses is render_refused.
func TestST_NoteTextsRefuses(t *testing.T) {
	ok := "rashomon record: gate 'go test' exited 1 on this run."
	model, user, refused := noteTexts(ok, false)
	if refused != nil || model != ok || user != render.MirrorPrefix+ok+render.MirrorSuffix {
		t.Errorf("noteTexts(valid) = %q, %q, %v", model, user, refused)
	}
	over := render.MessagePrefix + strings.Repeat("a", store.MaxSteerTextBytes)
	if _, _, refused := noteTexts(over, false); refused == nil || *refused != SuppressedByOutputGate {
		t.Errorf("an over-cap note is %v, want output_gate", refused)
	}
	if _, _, refused := noteTexts("rashomon record: you must run it.", false); refused == nil || *refused != SuppressedByRenderRefused {
		t.Errorf("a note Validate refuses is %v, want render_refused", refused)
	}
	briefing := render.MessagePrefix + "state of this session:\n- gate 'go test': last run exited 1."
	if _, user, refused := noteTexts(briefing, true); refused != nil || !strings.Contains(user, "\n") {
		t.Errorf("a briefing mirrors as %q, %v, want its lines kept", user, refused)
	}
	if _, _, refused := noteTexts(briefing, false); refused == nil || *refused != SuppressedByRenderRefused {
		t.Errorf("a multi-line text outside a briefing is %v, want render_refused", refused)
	}
	if user, refused := flagText(ok); refused != nil || user != ok {
		t.Errorf("flagText(valid) = %q, %v, want the bare text: a flag never says the agent was told", user, refused)
	}
	if _, refused := flagText(over); refused == nil || *refused != SuppressedByOutputGate {
		t.Errorf("an over-cap flag is %v, want output_gate", refused)
	}
}

// TestST_ChannelOfEveryEvent: the line's channel per event; PostToolUseFailure
// has no channel of its own, post serving it beside PostToolUse (tech 4.2.1).
func TestST_ChannelOfEveryEvent(t *testing.T) {
	want := map[Event]Channel{
		EventPreToolUse: ChannelPreToolUse, EventPostToolUse: ChannelPostToolUse,
		EventPostToolUseFailure: ChannelPostToolUse, EventPostToolBatch: ChannelPostToolBatch,
		EventUserPromptSubmit: ChannelUserPromptSubmit, EventSessionStart: ChannelSessionStart,
		EventSubagentStart: ChannelSubagentStart, EventStop: ChannelStop, EventPostModelSwitch: ChannelPostModelSwitch,
	}
	for _, e := range AllEvents() {
		if got := channelOf(e); got != want[e] {
			t.Errorf("channelOf(%s) = %q, want %q", e, got, want[e])
		}
	}
	if got := channelOf("NotAnEvent"); got != "" {
		t.Errorf("channelOf(unknown) = %q, want none", got)
	}
}

// TestST_NewIDIs32Hex: the default id is 32 hex characters, the
// invocation id's form, and two ids differ.
func TestST_NewIDIs32Hex(t *testing.T) {
	a, b := newID(), newID()
	if len(a) != 32 || strings.Trim(a, "0123456789abcdef") != "" || a == b {
		t.Errorf("newID() = %q, %q", a, b)
	}
}
