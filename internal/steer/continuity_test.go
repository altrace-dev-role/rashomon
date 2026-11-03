package steer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
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
// SessionStart, and at UserPromptSubmit, where a resumed session's first
// prompt gives it (D-c), and nowhere else, at no other priority, and with
// Check's every other rule; model-switch, the other own-channel id, is not
// admitted at SessionStart. SessionStart, like UserPromptSubmit, can have no call before
// it (a session that compacts before its first tool call), so there the
// briefing stands at after_seq 0, citing nothing, while a negative after_seq
// or evidence past it is refused. Break: admit it on every event, at any
// priority, refuse it at the prompt, keep the floor of 1 at SessionStart, or
// skip the seq check, and this fails.
func TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone(t *testing.T) {
	brief := Finding{Pattern: pattern.ContinuityBriefing, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 3}
	for _, ev := range []Event{EventSessionStart, EventUserPromptSubmit} {
		if err := brief.CheckOn(ev); err != nil {
			t.Errorf("the briefing at %s: %v, want admitted", ev, err)
		}
	}
	for _, ev := range []Event{EventPostToolUse, EventSubagentStart, EventPostToolBatch, EventStop, ""} {
		if brief.CheckOn(ev) == nil {
			t.Errorf("the briefing was admitted on %q", ev)
		}
	}
	if brief.Check() == nil {
		t.Error("Check, with no event, admitted the briefing")
	}
	first := brief
	first.AfterSeq = 0
	if err := first.CheckOn(EventSessionStart); err != nil {
		t.Errorf("the briefing at after_seq 0, a compaction before any call: %v, want admitted", err)
	}
	numbered := brief
	numbered.Priority = 5
	floor := brief
	floor.AfterSeq = -1
	past := brief
	past.Evidence = []int64{4}
	stop := brief
	stop.StopForm = StopFormDeny
	other := brief
	other.Pattern = pattern.ModelSwitch
	for name, f := range map[string]Finding{"priority 5": numbered, "after_seq -1": floor, "evidence past after_seq": past, "a stop form on a nudge": stop,
		"model-switch": other} {
		if f.CheckOn(EventSessionStart) == nil {
			t.Errorf("%s was admitted at SessionStart", name)
		}
	}
}

// TestCSL1_TheBriefingIsDirectOutsideThePromptBudget: SessionStart has no
// prompt, and the briefing is outside the prompt budget (tech 4.2.5: "1
// briefing per SessionStart or SubagentStart ... outside the prompt
// budget"; 4.2.13: "direct"). On its own event it is a nudge with no prompt
// and past a spent note cap, while a key already delivered is still
// budget_pattern and every other clamp still bites; any other finding with
// no prompt stays a flag, at SessionStart or on any other event, and so does
// the briefing on another event (the skip is narrow, mgr 11-02). Break: clamp
// the briefing for its missing prompt, charge it to the prompt's cap, skip
// the same-key check, or widen ownChannel to every pattern or every event,
// and this fails.
func TestCSL1_TheBriefingIsDirectOutsideThePromptBudget(t *testing.T) {
	m, err := render.New(pattern.ContinuityBriefing, render.ContinuitySaid1Binding{Said1: slot.NewSaid("they go vertical, we go horizontal", "")})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := render.New(pattern.ContinuityBriefing, render.ContinuitySaid1Binding{Said1: slot.NewSaidDigest(strings.Repeat("a", 64), 34, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	brief := Finding{Pattern: pattern.ContinuityBriefing, Key: "b-1", MaxLevel: LevelNudge, StopForm: StopFormNone, Message: m, Record: rec}
	c := verifiedClient
	c.StartSource = "compact"
	start := func() *Context {
		ctx := routerContext(t)
		ctx.Event, ctx.PromptID = EventSessionStart, nil
		return ctx
	}
	n := 0
	mint := func() string { n++; return strings.Repeat("0", 31) + string(rune('0'+n%10)) }
	spent := counts{notes: maxNotes, flags: maxFlags}

	out, lines := decide(start(), c, []Finding{brief}, spent, mint)
	if out.AdditionalContext != m.Text() || len(lines) != 1 || lines[0].State != LineStateDelivered || lines[0].Level != LevelNudge.String() {
		t.Errorf("the briefing at SessionStart, no prompt, the cap spent: %+v, %+v; want it delivered to the model", out, lines)
	}
	given := counts{keys: map[findingKey]bool{{string(pattern.ContinuityBriefing), "", "b-1"}: true}}
	if out, lines := decide(start(), c, []Finding{brief}, given, mint); out != (Output{}) || lines[0].SuppressedBy == nil ||
		*lines[0].SuppressedBy != SuppressedByBudgetPattern {
		t.Errorf("the briefing under a key already delivered: %+v, %+v; want budget_pattern", out, lines)
	}
	noConsent := start()
	noConsent.Consent = false
	if out, lines := decide(noConsent, c, []Finding{brief}, counts{}, mint); out.AdditionalContext != "" || lines[0].SuppressedBy == nil ||
		*lines[0].SuppressedBy != SuppressedByConsentMissing {
		t.Errorf("the briefing with no consent: %+v, %+v; want the consent clamp", out, lines)
	}
	post := start()
	post.Event = EventPostToolUse
	other := regressionNote(t, "go test", 5, "a", LevelNudge)
	for name, ev := range map[string]struct {
		ctx *Context
		f   Finding
	}{"another pattern at SessionStart": {start(), other}, "another pattern on PostToolUse": {post, other},
		"the briefing on PostToolUse": {post, brief}} {
		if out, lines := decide(ev.ctx, c, []Finding{ev.f}, counts{}, mint); out.AdditionalContext != "" || lines[0].SuppressedBy == nil ||
			*lines[0].SuppressedBy != SuppressedByBudgetPrompt {
			t.Errorf("%s with no prompt: %+v, %+v; want a flag, budget_prompt", name, out, lines)
		}
	}
}

// TestCSL1_AfterAResumeTheBriefingTakesThePromptsNote: a resumed session's
// first prompt gives the briefing its compaction did not (D-c), on its own
// channel there too: it takes the call's one note ahead of every ranked
// candidate, which an own-channel pattern never competes with
// (pattern.priority's doc), though it stands at priority 0, below them all;
// a task-preflight note of the same call is recorded superseded,
// budget_batch, never dropped (mgr 11-02); and the delivered briefing counts
// in none of the prompt's notes while it keeps its key (tech 4.2.5: "outside
// the prompt budget"), so the prompt's posts still have all three. Break:
// rank the briefing by its priority, drop the superseded line, or charge the
// briefing to the prompt's notes, and this fails.
func TestCSL1_AfterAResumeTheBriefingTakesThePromptsNote(t *testing.T) {
	m, err := render.New(pattern.ContinuityBriefing, render.ContinuitySaid1Binding{Said1: slot.NewSaid("they go vertical, we go horizontal", "")})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := render.New(pattern.ContinuityBriefing, render.ContinuitySaid1Binding{Said1: slot.NewSaidDigest(strings.Repeat("a", 64), 34, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	brief := Finding{Pattern: pattern.ContinuityBriefing, Key: "b-1", MaxLevel: LevelNudge, StopForm: StopFormNone, Message: m, Record: rec, AfterSeq: 1}
	pm, err := render.New(pattern.TaskPreflight, render.TaskPreflightBinding{
		Paths:    slot.NewPathSet([]slot.Path{"internal/verify/stop.go"}, 0, 0),
		Rules:    slot.NewRuleRefs(nil, 0, 0, 0),
		Gates:    slot.NewGateRefs([]slot.GateRef{{Words: "go test", Trust: slot.NewEnum(slot.EnumTrust, "trusted")}}, 0, -1),
		Source:   slot.NewEnum(slot.EnumEvidenceSource, "inferred from the request"),
		Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, []string{"judgment_disclosed"}),
		Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	pre := Finding{Pattern: pattern.TaskPreflight, Key: "note", Priority: 72, MaxLevel: LevelNudge, StopForm: StopFormNone, Message: pm, AfterSeq: 1}
	ctx := routerContext(t)
	ctx.Event = EventUserPromptSubmit
	out := Route(ctx, []Finding{pre, brief})
	if out.AdditionalContext != m.Text() {
		t.Errorf("the prompt's note = %q, want the briefing's %q", out.AdditionalContext, m.Text())
	}
	lines := steerLines(t, ctx)
	if len(lines) != 2 {
		t.Fatalf("%d lines, want the briefing's and the preflight's", len(lines))
	}
	for _, l := range lines {
		switch l.Pattern {
		case string(pattern.ContinuityBriefing):
			wantLine(t, l, LineStateDelivered, "", DeliveryDelivered)
		case string(pattern.TaskPreflight):
			if l.State != LineStateSuperseded || l.SuppressedBy == nil || *l.SuppressedBy != SuppressedByBudgetBatch {
				t.Errorf("the preflight's line: state %q, by %v; want superseded, budget_batch", l.State, l.SuppressedBy)
			}
		default:
			t.Errorf("a %q line", l.Pattern)
		}
	}
	log, err := os.ReadFile(filepath.Join(ctx.Store.RunDir(ctx.SessionID), store.FileSteer))
	if err != nil {
		t.Fatal(err)
	}
	if c := countOf(t, log, ctx.PromptID); c.notes != 0 || !c.keys[findingKey{string(pattern.ContinuityBriefing), "", "b-1"}] {
		t.Errorf("the prompt's counts after the briefing = %+v; want no note charged and the briefing's key", c)
	}
}
