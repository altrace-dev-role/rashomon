package steer

import (
	"slices"
	"testing"
)

func TestLevel_StringAndOrdering(t *testing.T) {
	want := map[Level]string{
		LevelOff: "off", LevelFlag: "flag", LevelNudge: "nudge", LevelStop: "stop",
	}
	for l, s := range want {
		if got := l.String(); got != s {
			t.Errorf("Level(%d).String() = %q, want %q", uint8(l), got, s)
		}
	}
	if !(LevelOff < LevelFlag && LevelFlag < LevelNudge && LevelNudge < LevelStop) {
		t.Error("Level ordering off < flag < nudge < stop does not hold")
	}
	if got := Level(200).String(); got != "unknown" {
		t.Errorf("Level(200).String() = %q, want %q", got, "unknown")
	}
}

// TestChannelsAndEvents pins both spellings as literals (tech 4.2.3's
// Channel list; Claude Code's hook event names), so a renamed constant fails
// here instead of moving its expected value with it. PostToolUseFailure is an
// event with no channel of its own: `post` serves it beside PostToolUse
// (tech 4.2.1).
func TestChannelsAndEvents(t *testing.T) {
	wantCh := []Channel{
		"pre_tool_use", "post_tool_use", "post_tool_batch", "user_prompt_submit",
		"session_start", "subagent_start", "stop", "post_model_switch",
	}
	if got := AllChannels(); !slices.Equal(got, wantCh) {
		t.Errorf("AllChannels() = %q, want %q", got, wantCh)
	}
	wantEv := []Event{
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PostToolBatch",
		"UserPromptSubmit", "SessionStart", "SubagentStart", "Stop", "PostModelSwitch",
	}
	if got := AllEvents(); !slices.Equal(got, wantEv) {
		t.Errorf("AllEvents() = %q, want %q", got, wantEv)
	}
}

// TestTRU13b_StateListCarriesUnconfirmed pins that LineState's eight values
// include "unconfirmed" -- the state a delivered line falls back to when
// its invocation is killed or dropped before anything could confirm
// delivery (Technical spec 4.2.3).
func TestTRU13b_StateListCarriesUnconfirmed(t *testing.T) {
	if LineStateUnconfirmed != "unconfirmed" {
		t.Fatalf("LineStateUnconfirmed = %q, want %q", LineStateUnconfirmed, "unconfirmed")
	}
	states := AllLineStates()
	if len(states) != 8 {
		t.Fatalf("AllLineStates() has %d entries, want 8", len(states))
	}
	for _, s := range states {
		if s == LineStateUnconfirmed {
			return
		}
	}
	t.Errorf("AllLineStates() = %v, missing %q", states, LineStateUnconfirmed)
}

// TestSuppressedBy_ExactlyTwentyThreeValues pins the closed list's size and
// exact membership: Technical spec 4.2.3's "the one closed list" of 22
// suppression reasons, and config_unreadable after them (STEER-CLI1: the hook
// clamps on a config.json it cannot read, and the person is told to fix the
// file, which steer_off's "run steer on" would not say).
func TestSuppressedBy_ExactlyTwentyThreeValues(t *testing.T) {
	all := AllSuppressedBy()
	if len(all) != 23 {
		t.Fatalf("AllSuppressedBy() has %d entries, want 23", len(all))
	}
	want := []SuppressedBy{
		"budget_prompt", "budget_pattern", "budget_batch", "stop_hook_active",
		"background_running", "host_foreign", "steer_off", "consent_missing",
		"arm_off", "demoted", "lock_timeout", "render_refused", "deadline",
		"deny_cap", "slot_rejected", "output_gate", "unverified_channel",
		"no_channel", "no_mirror", "harness_off", "surface_off", "late", "config_unreadable",
	}
	if len(want) != 23 {
		t.Fatalf("test setup: want has %d entries, not 23", len(want))
	}
	seen := make(map[SuppressedBy]bool, len(all))
	for i, v := range all {
		if v != want[i] {
			t.Errorf("AllSuppressedBy()[%d] = %q, want %q", i, v, want[i])
		}
		if seen[v] {
			t.Errorf("AllSuppressedBy() has duplicate %q", v)
		}
		seen[v] = true
	}
}

// TestClosedVocabulariesAreTheSpecsStrings pins every closed steer
// vocabulary by its literal strings, in the spec's order (tech, Contracts and
// names: Vocabularies; tech 4.2.3): a count alone lets a renamed value pass.
func TestClosedVocabulariesAreTheSpecsStrings(t *testing.T) {
	var levels []string
	for _, l := range AllLevels() {
		levels = append(levels, l.String())
	}
	cases := []struct {
		name      string
		got, want []string
	}{
		{"Level", levels, []string{"off", "flag", "nudge", "stop"}},
		{"Attendance", strs(AllAttendance()), []string{"attended", "unattended", "looping"}},
		{"Label", strs(AllLabels()), []string{"confirmed", "refuted", "unresolved"}},
		{"StopForm", strs(AllStopForms()), []string{"none", "deny", "ask", "stop_continue"}},
		{"PatternState", strs(AllPatternStates()), []string{"steering", "flag_only", "off_by_user"}},
		{"LineState", strs(AllLineStates()), []string{
			"candidate", "delivered", "suppressed", "superseded", "stale", "deferred", "expired", "unconfirmed",
		}},
		{"Delivery", strs(AllDelivery()), []string{"delivered", "flag_only", "holdout", "suppressed_budget", "shadow"}},
		{"Outcome", strs(AllOutcomes()), []string{"ask_unattended"}},
	}
	for _, c := range cases {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func strs[T ~string](vs []T) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

func TestOutcome_AskUnattended(t *testing.T) {
	if OutcomeAskUnattended != "ask_unattended" {
		t.Errorf("OutcomeAskUnattended = %q, want %q", OutcomeAskUnattended, "ask_unattended")
	}
}

// TestArmsCausesAndMirrorsAreTheSpecsLists pins the three vocabularies the
// spec closed on 2026-10-13: the arms (no placebo arm: E-9 moved it into the
// harness overlay), the pattern-state causes (archive 50.2.3) and the mirror
// channels and failures (archive 40.3.4).
func TestArmsCausesAndMirrorsAreTheSpecsLists(t *testing.T) {
	if got := AllArms(); !slices.Equal(got, []Arm{"live", "steer", "off"}) {
		t.Errorf("AllArms() = %q", got)
	}
	wantCauses := []Cause{"initial", "auto_demoted", "auto_repromoted", "flap_hold", "user_on", "user_off", "user_set_level", "version_changed"}
	if got := AllCauses(); !slices.Equal(got, wantCauses) {
		t.Errorf("AllCauses() = %q, want %q", got, wantCauses)
	}
	if got := AllMirrors(); !slices.Equal(got, []Mirror{"system_message", "steer_log", "notification", "sdk_informational", "none"}) {
		t.Errorf("AllMirrors() = %q", got)
	}
	if got := AllMirrorFailures(); !slices.Equal(got, []MirrorFailure{"failed", "killed", "busy"}) {
		t.Errorf("AllMirrorFailures() = %q", got)
	}
	for _, a := range []Arm{"placebo", ""} {
		if slices.Contains(AllArms(), a) {
			t.Errorf("arm %q must not exist", a)
		}
	}
}
