package steer

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestCSL1_ALineRecordsTheKeyedFormAndNeverTheWords: a steer line records a
// quote of the person's words only as its record form, the keyed digests and
// byte lengths, and refuses the words themselves, so a finding that rendered
// them with no Record is render_refused and its line never holds them (D160,
// a second guard behind Finding.Record). Break: record the plain form, or
// write the words beside the digest, and this fails.
func TestCSL1_ALineRecordsTheKeyedFormAndNeverTheWords(t *testing.T) {
	const words = "hes vertical we are horizontal and integration remember?"
	_, err := slotsFrom([]render.Slot{{Name: "said1", Value: slot.NewSaid(words, "they go vertical")}})
	if !errors.Is(err, errSlot) {
		t.Errorf("the person's words were recorded (err %v), want errSlot", err)
	}
	if err != nil && strings.Contains(err.Error(), words) {
		t.Errorf("the refusal names the words: %v", err)
	}
	d, e := strings.Repeat("c", 64), strings.Repeat("d", 64)
	got, err := slotsFrom([]render.Slot{{Name: "said1", Value: slot.NewSaidDigest(d, 57, e, 16)}})
	want := `{"bytes":57,"digest":"` + d + `","restates_bytes":16,"restates_digest":"` + e + `"}`
	if err != nil || string(got["said1"]) != want {
		t.Errorf("the record form writes %s (%v), want %s", got["said1"], err, want)
	}
}

// TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone: continuity-briefing, an
// own-channel pattern with no step 8 number, is admitted at priority 0 at
// SessionStart and nowhere else, at no other priority, and with Check's every
// other rule (after_seq's floor of 1 among them); model-switch, the other
// own-channel id, is not admitted at SessionStart. Break: admit it on every
// event, at any priority, or skip the seq floor, and this fails.
func TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone(t *testing.T) {
	brief := Finding{Pattern: pattern.ContinuityBriefing, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 3}
	if err := brief.CheckOn(EventSessionStart); err != nil {
		t.Errorf("the briefing at SessionStart: %v, want admitted", err)
	}
	for _, ev := range []Event{EventPostToolUse, EventUserPromptSubmit, EventSubagentStart, EventPostToolBatch, ""} {
		if brief.CheckOn(ev) == nil {
			t.Errorf("the briefing was admitted on %q", ev)
		}
	}
	if brief.Check() == nil {
		t.Error("Check, with no event, admitted the briefing")
	}
	numbered := brief
	numbered.Priority = 5
	floor := brief
	floor.AfterSeq = 0
	stop := brief
	stop.StopForm = StopFormDeny
	other := brief
	other.Pattern = pattern.ModelSwitch
	for name, f := range map[string]Finding{"priority 5": numbered, "after_seq 0": floor, "a stop form on a nudge": stop,
		"model-switch": other} {
		if f.CheckOn(EventSessionStart) == nil {
			t.Errorf("%s was admitted at SessionStart", name)
		}
	}
}
