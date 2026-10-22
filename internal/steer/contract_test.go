package steer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestDEL22c_PatternInterfaceIsTheProposal pins the pattern interface P-07
// proposes for the gap row "The pattern interface, including Revalidate"
// (tech, Contracts and names): exactly ID and Revalidate, with these
// signatures. Which evaluates a label is MEASURE's label_rule table (MSR 13,
// decided (a)), so the interface has no Label method.
func TestDEL22c_PatternInterfaceIsTheProposal(t *testing.T) {
	typ := reflect.TypeOf((*Pattern)(nil)).Elem()
	want := map[string]string{
		"ID":         "func() pattern.ID",
		"Revalidate": "func(*steer.Context, steer.Finding) bool",
	}
	if typ.NumMethod() != len(want) {
		t.Errorf("Pattern has %d methods, want %d", typ.NumMethod(), len(want))
	}
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		sig, ok := want[m.Name]
		if !ok {
			t.Errorf("Pattern has the method %s, which the proposal does not name", m.Name)
			continue
		}
		if got := m.Type.String(); got != sig {
			t.Errorf("Pattern.%s is %s, want %s", m.Name, got, sig)
		}
	}
}

// TestDEL22d_SteerLinePromptIDIsNullable: a finding with no prompt (a
// SessionStart briefing, a flag-only finding, tech 4.2.5) writes
// "prompt_id":null, and null reads back as no prompt, never as "".
func TestDEL22d_SteerLinePromptIDIsNullable(t *testing.T) {
	b, err := json.Marshal(Line{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"prompt_id":null`) {
		t.Errorf("the zero line writes %s, want \"prompt_id\":null", b)
	}
	var l Line
	if err := json.Unmarshal([]byte(`{"prompt_id":null}`), &l); err != nil {
		t.Fatal(err)
	}
	if !reflect.ValueOf(l.PromptID).IsZero() {
		t.Errorf("prompt_id null decoded as %v, want no prompt", l.PromptID)
	}
	if err := json.Unmarshal([]byte(`{"prompt_id":"p1"}`), &l); err != nil {
		t.Fatal(err)
	}
	if l.PromptID == nil || *l.PromptID != "p1" {
		t.Errorf("prompt_id \"p1\" decoded as %v", l.PromptID)
	}
}

// TestDEL22e_FindingCheckHoldsTheFrozenKinds: Check refuses a Finding whose
// kind is not one of its pattern's frozen kinds (tech 72: "only frozen kinds
// cross"), a model-visible claim-vs-record/masked (ED-3) or user-flag
// pattern, a stop form without a stop level, and evidence out of order or
// after AfterSeq.
func TestDEL22e_FindingCheckHoldsTheFrozenKinds(t *testing.T) {
	ok := []Finding{
		{Pattern: pattern.ClaimVsRecord, Kind: "red", Priority: 85, MaxLevel: LevelStop, StopForm: StopFormStopContinue, Evidence: []int64{37, 48}, AfterSeq: 57},
		{Pattern: pattern.ClaimVsRecord, Kind: "masked", Priority: 85, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.SubtaskGate, Kind: "", Priority: 90, MaxLevel: LevelStop, StopForm: StopFormDeny, AfterSeq: 5},
		{Pattern: pattern.SubtaskGate, Kind: "commit", Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.PlatformRule, Kind: "edit", Priority: 60, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.Integrity, Kind: "test-deleted", Priority: 95, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.OrderingRule, Priority: 30, MaxLevel: LevelStop, StopForm: StopFormAsk, AfterSeq: 5},
		{Pattern: pattern.DeclaredFact, Kind: "auth.expired", Priority: 75, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
	}
	for _, f := range ok {
		if err := f.Check(); err != nil {
			t.Errorf("%s/%s: %v", f.Pattern, f.Kind, err)
		}
	}
	// Every case below carries its pattern's own correct Priority (and an
	// AfterSeq of at least 1) unless the case is testing the priority or
	// after_seq check itself. Leaving either at its zero value makes that
	// check fail the case for a reason unrelated to what the case names,
	// which hides a regression in the check the case is actually for.
	bad := []Finding{
		{Pattern: "purple", MaxLevel: LevelFlag, StopForm: StopFormNone},
		{Pattern: pattern.ClaimVsRecord, Kind: "masked_inconclusive", Priority: 85, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.ClaimVsRecord, Kind: "", Priority: 85, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.ClaimVsRecord, Kind: "masked", Priority: 85, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.ClaimVsRecord, Kind: "masked", Priority: 85, MaxLevel: LevelStop, StopForm: StopFormStopContinue, AfterSeq: 5},
		{Pattern: pattern.PlatformRule, Kind: "forbid_code", Priority: 60, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.Integrity, Kind: "skip_added", Priority: 95, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.Integrity, Kind: "skip-added", Priority: 95, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.SequenceRisk, Kind: "secret-then-egress", Priority: 62, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.ModelSwitch, MaxLevel: LevelNudge, StopForm: StopFormNone},
		{Pattern: pattern.RegressionLocation, Kind: "red", Priority: 80, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.DeclaredFact, Kind: "", Priority: 75, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.OrderingRule, Priority: 30, MaxLevel: LevelNudge, StopForm: StopFormAsk, AfterSeq: 5},
		{Pattern: pattern.OrderingRule, Priority: 30, MaxLevel: LevelStop, StopForm: "hold", AfterSeq: 5},
		{Pattern: pattern.OrderingRule, Priority: 30, MaxLevel: LevelStop + 1, StopForm: StopFormAsk, AfterSeq: 5},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, Evidence: []int64{52, 37}, AfterSeq: 57},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, Evidence: []int64{37, 37}, AfterSeq: 57},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, Evidence: []int64{37, 58}, AfterSeq: 57},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, Evidence: []int64{0}, AfterSeq: 5},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, Evidence: []int64{-5, 0}, AfterSeq: 10},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 0},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: -1},
		{Pattern: pattern.SubtaskGate, Priority: 999, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.ContinuityBriefing, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
		{Pattern: pattern.ModelSwitch, MaxLevel: LevelFlag, StopForm: StopFormNone, AfterSeq: 5},
	}
	for _, f := range bad {
		if err := f.Check(); err == nil {
			t.Errorf("Check accepted %s/%q level %v form %q evidence %v after %d", f.Pattern, f.Kind, f.MaxLevel, f.StopForm, f.Evidence, f.AfterSeq)
		}
	}
}
