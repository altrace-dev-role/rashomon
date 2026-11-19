package steer

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// hold puts pairs in heldAtFlag for one test. The table holds no row until
// a pattern is held, so each test holds its own fixture pairs.
func hold(t *testing.T, pairs ...Pair) {
	t.Helper()
	was := heldAtFlag
	heldAtFlag = pairs
	t.Cleanup(func() { heldAtFlag = was })
}

var (
	regressionPair = Pair{Pattern: pattern.RegressionLocation}
	stalePair      = Pair{Pattern: pattern.StaleEvidence}
	preflightPair  = Pair{Pattern: pattern.TaskPreflight}
)

// routed routes fs on a fresh routerContext that set, when not nil, has
// changed, and returns what printed and the steer log's lines as written.
// Two calls with the same set and findings write the same bytes: the ids
// are counted and the clock is fixed.
func routed(t *testing.T, set func(*Context), fs ...Finding) (Output, [][]byte) {
	t.Helper()
	ctx := routerContext(t)
	if set != nil {
		set(ctx)
	}
	out := Route(ctx, fs)
	raw, skipped, err := ctx.Store.ReadSteer(ctx.SessionID)
	if err != nil || skipped != 0 {
		t.Fatalf("ReadSteer: %v, %d skipped", err, skipped)
	}
	return out, raw
}

// sameRoute fails unless two routings printed the same Output and wrote the
// same lines, byte for byte.
func sameRoute(t *testing.T, what string, out Output, log [][]byte, wantOut Output, wantLog [][]byte) {
	t.Helper()
	if out != wantOut || len(log) != len(wantLog) {
		t.Errorf("%s: printed %+v with %d lines, want %+v with %d", what, out, len(log), wantOut, len(wantLog))
		return
	}
	for i := range log {
		if !bytes.Equal(log[i], wantLog[i]) {
			t.Errorf("%s: line %d is\n%s\nwant\n%s", what, i, log[i], wantLog[i])
		}
	}
}

// atLevel is f produced at level l.
func atLevel(f Finding, l Level) Finding {
	f.MaxLevel = l
	return f
}

// asideOnlyNote is a task-preflight note that lists nothing proven, so it
// has no text for the model and an aside for the person (TOKEN-BUDGET1).
func asideOnlyNote(t *testing.T) Finding {
	t.Helper()
	m, err := render.New(pattern.TaskPreflight, render.TaskPreflightBinding{
		Paths:    slot.NewPathSet(nil, 0, 2),
		Rules:    slot.NewRuleRefs(nil, 0, 0, 0),
		Gates:    slot.NewGateRefs(nil, 0, -1),
		Source:   slot.NewEnum(slot.EnumEvidenceSource, "inferred from the request"),
		Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, []string{"judgment_disclosed"}),
		Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, []string{"no spec found"}),
	})
	if err != nil || m.Text() != "" || m.Aside() == "" {
		t.Fatalf("premise: render.New(task-preflight) = text %q, aside %q, %v; want an aside alone", m.Text(), m.Aside(), err)
	}
	return Finding{Pattern: pattern.TaskPreflight, Key: "note", Priority: 72, MaxLevel: LevelNudge, StopForm: StopFormNone,
		Message: m, AfterSeq: 1}
}

// TestRule2_HeldPairStaysAtFlagByDefault: a held pair's note prints as a
// flag, and its line is the line of the same note produced at flag, byte
// for byte, with no reason on it, so no reader can tell the hold from the
// finding's own level. A stop-level note is held too, since the hold
// follows the stop clamp. Unheld, the note reaches the model. Break: drop
// the hold, or test it before the stop clamp.
func TestRule2_HeldPairStaysAtFlagByDefault(t *testing.T) {
	note := regressionNote(t, "go test", 5, "a", LevelNudge)
	if out, _ := routed(t, nil, note); out.AdditionalContext != note.Message.Text() {
		t.Fatalf("premise: unheld, the note printed %+v, want its text to the model", out)
	}
	flagOut, flagLog := routed(t, nil, atLevel(note, LevelFlag))
	if flagOut.AdditionalContext != "" || flagOut.SystemMessage == "" {
		t.Fatalf("premise: the note produced at flag printed %+v, want a flag", flagOut)
	}
	hold(t, regressionPair)
	for _, l := range []Level{LevelNudge, LevelStop} {
		out, log := routed(t, nil, atLevel(note, l))
		sameRoute(t, "held at "+l.String(), out, log, flagOut, flagLog)
	}
}

// TestRule2_OverrideRaisesOnlyItsPair: with two pairs held, the override
// lets through the pair it names, whose note then routes as it would
// unheld, byte for byte, and no other: the other held pair's note stays a
// flag, and an override naming the pattern under another kind, another
// pattern, or nothing raises nothing. Break: ignore the override, or
// compare its pattern or its kind alone.
func TestRule2_OverrideRaisesOnlyItsPair(t *testing.T) {
	regression := regressionNote(t, "go test", 5, "a", LevelNudge)
	stale := staleNote(t, "make", 5)
	freeOut, freeLog := routed(t, nil, regression)
	if freeOut.AdditionalContext != regression.Message.Text() {
		t.Fatalf("premise: unheld, the note printed %+v, want its text to the model", freeOut)
	}
	flagOut, flagLog := routed(t, nil, atLevel(regression, LevelFlag))
	staleFlagOut, staleFlagLog := routed(t, nil, atLevel(stale, LevelFlag))
	if staleFlagOut.AdditionalContext != "" || staleFlagOut.SystemMessage == "" {
		t.Fatalf("premise: the stale note produced at flag printed %+v, want a flag", staleFlagOut)
	}
	override := func(p Pair) func(*Context) {
		return func(ctx *Context) { ctx.LevelOverride = p }
	}
	hold(t, regressionPair, stalePair)

	out, log := routed(t, override(regressionPair), regression)
	sameRoute(t, "the named pair", out, log, freeOut, freeLog)
	out, log = routed(t, override(regressionPair), stale)
	sameRoute(t, "the other held pair", out, log, staleFlagOut, staleFlagLog)
	for _, near := range []Pair{{Pattern: pattern.RegressionLocation, Kind: "other"}, stalePair, {}} {
		out, log := routed(t, override(near), regression)
		sameRoute(t, fmt.Sprintf("override %+v", near), out, log, flagOut, flagLog)
	}
}

// TestRule2_OverrideKeepsEveryOtherClamp: the override lifts the hold and
// nothing else. Under each clamp that lowers a note to a flag (an
// unreadable config, steering off, no consent, no prompt yet, the hook
// already active at a Stop, an event with no model channel) and for a note
// with nothing for the model, the held pair under its override routes as
// the same note unheld, byte for byte, and gives the model nothing. Break:
// let the override skip any of those clamps.
func TestRule2_OverrideKeepsEveryOtherClamp(t *testing.T) {
	note := regressionNote(t, "go test", 5, "a", LevelNudge)
	clamps := []struct {
		name string
		f    Finding
		set  func(*Context)
	}{
		{"config unreadable", note, func(c *Context) { c.ConfigUnreadable = true }},
		{"steer off", note, func(c *Context) { c.SteerOff = true }},
		{"no consent", note, func(c *Context) { c.Consent = false }},
		{"no prompt", note, func(c *Context) { c.PromptID = nil }},
		{"stop hook active", note, func(c *Context) { c.Event, c.StopHookActive = EventStop, true }},
		{"no model channel", note, func(c *Context) { c.Event = EventStop }},
		{"nothing for the model", asideOnlyNote(t), func(c *Context) { c.Event = EventUserPromptSubmit }},
	}
	type routing struct {
		out Output
		log [][]byte
	}
	free := make([]routing, len(clamps))
	for i, c := range clamps {
		out, log := routed(t, c.set, c.f)
		if out.AdditionalContext != "" {
			t.Fatalf("premise: under %s the unheld note reached the model: %+v", c.name, out)
		}
		free[i] = routing{out, log}
	}
	hold(t, regressionPair, preflightPair)
	for i, c := range clamps {
		pair := Pair{Pattern: c.f.Pattern, Kind: c.f.Kind}
		out, log := routed(t, func(ctx *Context) { c.set(ctx); ctx.LevelOverride = pair }, c.f)
		sameRoute(t, c.name, out, log, free[i].out, free[i].log)
	}
}
