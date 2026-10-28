package steer

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// TestMS5b_HeldOutNoteReachesTheUserOnly: in a session drawn into the field
// holdout (card 2.5, D109) the call's note goes to the user only: nothing in
// additionalContext, and a systemMessage that is the note's text with the
// held-out suffix, never the mirror wrapper, which says the agent was told
// (G1). The line records delivery holdout with what each side got: no model
// text, the printed user text, mirrored to the user.
func TestMS5b_HeldOutNoteReachesTheUserOnly(t *testing.T) {
	ctx := routerContext(t)
	ctx.Holdout = true
	f := regressionNote(t, "go test", 5, "a", LevelNudge)
	out := Route(ctx, []Finding{f})
	if out.AdditionalContext != "" || out.SystemMessage != f.Message.Text()+holdoutSuffix || out.Decision != nil || out.StopContinue != "" {
		t.Errorf("Output = %+v, want no additionalContext and the note's text ending in %q", out, holdoutSuffix)
	}
	if !strings.HasSuffix(out.SystemMessage, holdoutSuffix) || strings.HasPrefix(out.SystemMessage, render.MirrorPrefix) {
		t.Errorf("systemMessage %q: want the held-out suffix and not the mirror's %q", out.SystemMessage, render.MirrorPrefix)
	}
	l := lastLines(t, ctx, 1)[0]
	wantLine(t, l, LineStateDelivered, "", DeliveryHoldout)
	if l.Level != LevelNudge.String() || l.ModelText != "" || l.UserText != out.SystemMessage || l.Mirror != MirrorSystemMessage || !l.MirroredToUser {
		t.Errorf("line level %q, texts %q / %q, mirror %q, mirrored %v; want nudge, no model text, the systemMessage, %q, true",
			l.Level, l.ModelText, l.UserText, l.Mirror, l.MirroredToUser, MirrorSystemMessage)
	}
}

// TestMS5c_FlagsAreNeverHeldOut: only the call's note is held out (R3). A
// flag, whether its finding is one, a clamp made it one or its event has no
// model channel, prints the same bytes in a holdout session as in a treated
// one, and its line is flag_only.
func TestMS5c_FlagsAreNeverHeldOut(t *testing.T) {
	for _, tc := range []struct {
		name  string
		level Level
		edit  func(*Context)
	}{
		{"flag level", LevelFlag, func(*Context) {}},
		{"consent clamp", LevelNudge, func(c *Context) { c.Consent = false }},
		{"stop event", LevelNudge, func(c *Context) { c.Event = EventStop }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			route := func(holdout bool) ([]byte, Line) {
				ctx := routerContext(t)
				tc.edit(ctx)
				ctx.Holdout = holdout
				out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", tc.level)})
				b, err := out.MarshalFor(string(ctx.Event), ctx.Client)
				if err != nil || len(b) == 0 {
					t.Fatalf("MarshalFor = %q, %v; want the flag's bytes", b, err)
				}
				return b, lastLines(t, ctx, 1)[0]
			}
			treated, tl := route(false)
			held, hl := route(true)
			if string(held) != string(treated) {
				t.Errorf("held out prints %s, treated prints %s; want the same bytes", held, treated)
			}
			by := func(l Line) SuppressedBy {
				if l.SuppressedBy == nil {
					return ""
				}
				return *l.SuppressedBy
			}
			wantLine(t, hl, LineStateDelivered, by(tl), DeliveryFlagOnly)
			if hl.UserText != tl.UserText || hl.ModelText != "" {
				t.Errorf("held-out line's texts %q / %q, treated user text %q", hl.ModelText, hl.UserText, tl.UserText)
			}
		})
	}
}
