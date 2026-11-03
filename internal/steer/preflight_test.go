package steer

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestPF1r_PreflightNoteIsMirroredAcrossItsLines: task-preflight is the
// second multi-line note, so a UserPromptSubmit preflight delivers as
// additionalContext with MirrorBriefing's mirror, not render_refused by the
// one-line Mirror, and the note's aside, the lines the model is not sent,
// prints after the mirror in the same systemMessage (TOKEN-BUDGET1); the
// steer line keeps both texts as they printed. Break: decide passes the
// briefing flag for continuity-briefing only, and the note never prints; or
// the aside is dropped, or sent to the model.
func TestPF1r_PreflightNoteIsMirroredAcrossItsLines(t *testing.T) {
	m, err := render.New(pattern.TaskPreflight, render.TaskPreflightBinding{
		Paths:    slot.NewPathSet([]slot.Path{"internal/verify/stop.go"}, 0, 0),
		Rules:    slot.NewRuleRefs(nil, 0, 0, 0),
		Gates:    slot.NewGateRefs([]slot.GateRef{{Words: "go test", Trust: slot.NewEnum(slot.EnumTrust, "trusted")}}, 0, -1),
		Source:   slot.NewEnum(slot.EnumEvidenceSource, "inferred from the request"),
		Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, []string{"judgment_disclosed"}),
		Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, nil),
	})
	if err != nil {
		t.Fatalf("premise: render.New(task-preflight): %v", err)
	}
	ctx := routerContext(t)
	ctx.Event = EventUserPromptSubmit
	f := Finding{Pattern: pattern.TaskPreflight, Key: "note", Priority: 72, MaxLevel: LevelNudge, StopForm: StopFormNone, Message: m, AfterSeq: 1}
	out := Route(ctx, []Finding{f})
	mirror, err := render.MirrorBriefing(m.Text())
	if err != nil {
		t.Fatal(err)
	}
	if m.Aside() == "" {
		t.Error("the note has no aside: its evidence line is the person's")
	}
	user := mirror + "\n" + render.AsidePrefix + m.Aside()
	if out.AdditionalContext != m.Text() || out.SystemMessage != user {
		t.Errorf("Output = %+v, want the note, and its briefing mirror with the aside after it", out)
	}
	lines := steerLines(t, ctx)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}
	wantLine(t, lines[0], LineStateDelivered, "", DeliveryDelivered)
	if lines[0].ModelText != m.Text() || lines[0].UserText != user {
		t.Errorf("the line keeps model %q, user %q; want what printed", lines[0].ModelText, lines[0].UserText)
	}
}

// TestTB1_ANoteWithNothingForTheModelPrintsItsAsideAlone: a preflight whose
// lines list nothing proven has no model text (TOKEN-BUDGET1), so it prints
// as a flag: the agent gets 0 bytes, and the person gets the aside, without
// the mirror, which would say the agent was told. Break: route it as a note
// (an empty additionalContext and a mirror of nothing), or suppress it as
// render_refused, so the person never sees what was not known.
func TestTB1_ANoteWithNothingForTheModelPrintsItsAsideAlone(t *testing.T) {
	m, err := render.New(pattern.TaskPreflight, render.TaskPreflightBinding{
		Paths:    slot.NewPathSet(nil, 0, 2),
		Rules:    slot.NewRuleRefs(nil, 0, 0, 0),
		Gates:    slot.NewGateRefs(nil, 0, -1),
		Source:   slot.NewEnum(slot.EnumEvidenceSource, "inferred from the request"),
		Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, []string{"judgment_disclosed"}),
		Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, []string{"no spec found"}),
	})
	if err != nil {
		t.Fatalf("premise: render.New(task-preflight): %v", err)
	}
	if m.Text() != "" || m.Aside() == "" {
		t.Errorf("render.New: text %q, aside %q; want no model text and an aside", m.Text(), m.Aside())
	}
	ctx := routerContext(t)
	ctx.Event = EventUserPromptSubmit
	f := Finding{Pattern: pattern.TaskPreflight, Key: "note", Priority: 72, MaxLevel: LevelNudge, StopForm: StopFormNone, Message: m, AfterSeq: 1}
	out := Route(ctx, []Finding{f})
	if want := render.AsidePrefix + m.Aside(); out.AdditionalContext != "" || out.SystemMessage != want {
		t.Errorf("Output = %+v, want no model text and the systemMessage %q", out, want)
	}
	lines := steerLines(t, ctx)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1", len(lines))
	}
	wantLine(t, lines[0], LineStateDelivered, "", DeliveryFlagOnly)
	if lines[0].Level != LevelFlag.String() || lines[0].ModelText != "" || lines[0].UserText != out.SystemMessage {
		t.Errorf("the line is level %q, model %q, user %q; want a flag that keeps what printed", lines[0].Level, lines[0].ModelText, lines[0].UserText)
	}
}

// TestPF1t_SeqZeroIsAdmittedOnAPromptOnly: UserPromptSubmit is the one event
// with no call before it, so at a session's first prompt the run has no
// record and the last seq the decision saw is 0. A finding routed on that
// event may stand at after_seq 0, with no evidence; on every other event,
// and with no context at all, after_seq stays at least 1. The event is the
// hook's Context's, never the producer's word. Break: admit keeps Check's
// rule on every event, and prompt 1 never gets its note.
func TestPF1t_SeqZeroIsAdmittedOnAPromptOnly(t *testing.T) {
	zero := Finding{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone}
	if err := zero.CheckOn(EventUserPromptSubmit); err != nil {
		t.Errorf("CheckOn(UserPromptSubmit) refused after_seq 0: %v", err)
	}
	for _, e := range []Event{EventPostToolUse, EventPreToolUse, EventStop, ""} {
		if zero.CheckOn(e) == nil {
			t.Errorf("CheckOn(%q) admitted after_seq 0", e)
		}
	}
	if zero.Check() == nil {
		t.Error("Check admitted after_seq 0 with no event")
	}
	for _, f := range []Finding{
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: -1},
		{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, Evidence: []int64{1}},
		{Pattern: pattern.SubtaskGate, Kind: "not-a-kind", Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone},
	} {
		if f.CheckOn(EventUserPromptSubmit) == nil {
			t.Errorf("CheckOn(UserPromptSubmit) admitted after %d, evidence %v, kind %q", f.AfterSeq, f.Evidence, f.Kind)
		}
	}
	if got := admit(&Context{Event: EventUserPromptSubmit}, []Finding{zero}); len(got) != 1 {
		t.Errorf("admit on UserPromptSubmit kept %d of the after_seq 0 finding, want 1", len(got))
	}
	if got := admit(&Context{Event: EventPostToolUse}, []Finding{zero}); len(got) != 0 {
		t.Errorf("admit on PostToolUse kept the after_seq 0 finding")
	}
	if got := admit(nil, []Finding{zero}); len(got) != 0 {
		t.Errorf("admit with no context kept the after_seq 0 finding")
	}
}
