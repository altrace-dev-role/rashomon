package steer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestMS5b_HeldOutNoteReachesNeitherSide: in a session drawn into the field
// holdout (card 2.5, D109) the call's note goes to neither side: nothing in
// additionalContext, and neither the mirror, which says the agent was told
// (G1), nor the note's text in the systemMessage. The session's first
// held-out note prints the holdout notice in its place, once; a later one
// prints nothing (card VERSION-GATE1). Each line records delivery holdout
// with no text and no mirror, and the first records the notice.
func TestMS5b_HeldOutNoteReachesNeitherSide(t *testing.T) {
	ctx := routerContext(t)
	ctx.Holdout = true
	first := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)})
	if first != (Output{SystemMessage: holdoutNotice}) {
		t.Errorf("first held-out call printed %+v, want only the holdout notice %q", first, holdoutNotice)
	}
	if second := Route(ctx, []Finding{regressionNote(t, "go vet", 6, "b", LevelNudge)}); second != (Output{}) {
		t.Errorf("second held-out call printed %+v, want nothing", second)
	}
	lines := lastLines(t, ctx, 2)
	for i, l := range lines {
		wantLine(t, l, LineStateDelivered, "", DeliveryHoldout)
		if l.Level != LevelNudge.String() || l.ModelText != "" || l.UserText != "" || l.Mirror != MirrorNone || l.MirroredToUser {
			t.Errorf("line %d: level %q, texts %q / %q, mirror %q, mirrored %v; want nudge, no text, %q, false",
				i, l.Level, l.ModelText, l.UserText, l.Mirror, l.MirroredToUser, MirrorNone)
		}
	}
	if n := lines[0].Notice; n == nil || *n != NoticeHoldout {
		t.Errorf("first line's notice = %v, want %q", n, NoticeHoldout)
	}
	if n := lines[1].Notice; n != nil {
		t.Errorf("second line's notice = %q, want null", *n)
	}
	if _, err := os.Lstat(filepath.Join(ctx.Store.RunDir(ctx.SessionID), store.FileNoticeHoldout)); err != nil {
		t.Errorf("the holdout marker: %v", err)
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
			if hl.UserText != tl.UserText || hl.ModelText != "" || hl.Notice != nil {
				t.Errorf("held-out line's texts %q / %q, notice %v, treated user text %q", hl.ModelText, hl.UserText, hl.Notice, tl.UserText)
			}
		})
	}
}
