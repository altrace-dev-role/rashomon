package hook

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// twinST is a fixture whose store holds f's install key. A line's learned
// gate ids and digests are HMACs under its store's key (shape.CWDDigest),
// and each new store draws its own, so an arm's line can equal the live
// line only when both stores share one.
func twinST(t *testing.T, f *stFix) *stFix {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, store.FileInstallKey), []byte(hex.EncodeToString(f.st.Key())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return newSTIn(t, root)
}

// TestMS1_ArmOffLogsShadowAndPrintsNothing: under arm off the step decides
// the regression as it would live and prints nothing. Its one line is the
// counterfactual: arm off, suppressed by arm_off, delivery shadow, with the
// live line's pattern, key and slots. The control, the same calls with no
// arm, prints the consent flag.
func TestMS1_ArmOffLogsShadowAndPrintsNothing(t *testing.T) {
	control := newST(t)
	if b := control.red("toolu_r").Steer(stEnv(0)); !printsFlag(t, b) {
		t.Fatalf("premise: the control printed %q, want the consent flag", b)
	}
	live := steerLines(t, control.st)

	f := twinST(t, control)
	env := stEnv(0)
	env.Arm = SteerArm("off")
	if b := f.red("toolu_r").Steer(env); b != nil {
		t.Errorf("Steer under arm off = %q, want nil", b)
	}
	got := steerLines(t, f.st)
	if len(live) != 1 || len(got) != 1 {
		t.Fatalf("the control logged %d lines and arm off %d, want 1 each", len(live), len(got))
	}
	l := got[0]
	if l.Arm != steer.ArmOff {
		t.Errorf("arm = %q, want %q", l.Arm, steer.ArmOff)
	}
	if l.State != steer.LineStateSuppressed || l.SuppressedBy == nil || *l.SuppressedBy != steer.SuppressedByArmOff {
		t.Errorf("state %q suppressed_by %v, want suppressed by %q", l.State, l.SuppressedBy, steer.SuppressedByArmOff)
	}
	if l.Delivery == nil || *l.Delivery != steer.DeliveryShadow {
		t.Errorf("delivery = %v, want %q", l.Delivery, steer.DeliveryShadow)
	}
	if l.Pattern != live[0].Pattern || l.Key != live[0].Key || len(l.Slots) == 0 || len(l.Slots) != len(live[0].Slots) {
		t.Errorf("pattern %q key %q with %d slots, want the live line's %q %q with %d",
			l.Pattern, l.Key, len(l.Slots), live[0].Pattern, live[0].Key, len(live[0].Slots))
	}
	for k, v := range live[0].Slots {
		if !bytes.Equal(l.Slots[k], v) {
			t.Errorf("slot %s = %s, want the live line's %s", k, l.Slots[k], v)
		}
	}
}

// TestMS1b_ArmSteerDeliversAsLive: arm steer prints what live prints, and
// its line differs from the live line only in the arm it names.
func TestMS1b_ArmSteerDeliversAsLive(t *testing.T) {
	control := newST(t)
	want := control.red("toolu_r").Steer(stEnv(0))
	if !printsFlag(t, want) {
		t.Fatalf("premise: the control printed %q, want the consent flag", want)
	}
	f := twinST(t, control)
	env := stEnv(0)
	env.Arm = SteerArm("steer")
	if b := f.red("toolu_r").Steer(env); !bytes.Equal(b, want) {
		t.Errorf("Steer under arm steer = %q, want the live output %q", b, want)
	}
	live, got := steerLines(t, control.st), steerLines(t, f.st)
	if len(live) != 1 || len(got) != 1 {
		t.Fatalf("the control logged %d lines and arm steer %d, want 1 each", len(live), len(got))
	}
	if got[0].Arm != steer.ArmSteer || live[0].Arm != steer.ArmLive {
		t.Errorf("arms %q and %q, want %q under steer and %q live", got[0].Arm, live[0].Arm, steer.ArmSteer, steer.ArmLive)
	}
	g, w := got[0], live[0]
	if g.State != w.State || !samePtr(g.SuppressedBy, w.SuppressedBy) || !samePtr(g.Delivery, w.Delivery) ||
		g.Level != w.Level || g.UserText != w.UserText || g.ModelText != w.ModelText || g.Key != w.Key {
		t.Errorf("arm steer's line\n%+v\nwant the live line\n%+v", g, w)
	}
}

func samePtr[T comparable](a, b *T) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// TestMS1c_UnknownArmIsLive: the operator's arm is "off" or "steer"
// exactly. Every other value, "live" among them, is no arm, and contextOf
// then names live, as it did before the switch.
func TestMS1c_UnknownArmIsLive(t *testing.T) {
	for v, want := range map[string]steer.Arm{
		"off": steer.ArmOff, "steer": steer.ArmSteer,
		"": "", "live": "", "OFF": "", "Steer": "", " off": "", "on": "", "0": "",
	} {
		if got := SteerArm(v); got != want {
			t.Errorf("SteerArm(%q) = %q, want %q", v, got, want)
		}
	}
	st := mustOpen(t)
	for _, tc := range []struct {
		env  steer.Arm
		want steer.Arm
	}{{"", steer.ArmLive}, {steer.ArmOff, steer.ArmOff}, {steer.ArmSteer, steer.ArmSteer}} {
		env := stEnv(steer.DisclosureVersion)
		env.Arm = tc.env
		p := &Post{st: st, sessionID: "s1", promptID: "p1", permissionMode: "default", event: FailureEvent, cwd: "/tmp/project"}
		if got := p.contextOf(env, verify.SteerView{ProjectRoot: "/repo"}).Arm; got != tc.want {
			t.Errorf("contextOf under env arm %q names %q, want %q", tc.env, got, tc.want)
		}
	}
}

// TestMS1d_AnArmSkipsTheHoldoutDraw: a session the field holdout draws is
// treatment under either forced arm, so a forced arm's lines never carry the
// holdout delivery and the arm field alone tells a forced run from a drawn
// one.
func TestMS1d_AnArmSkipsTheHoldoutDraw(t *testing.T) {
	st := mustOpen(t)
	held, _ := armSessions(t, st)
	env := stEnv(steer.DisclosureVersion)
	if !drawnIn(st, env, held) {
		t.Fatalf("premise: session %s is not drawn with no arm", held)
	}
	for _, a := range []steer.Arm{steer.ArmOff, steer.ArmSteer} {
		env.Arm = a
		if drawnIn(st, env, held) {
			t.Errorf("session %s is drawn under arm %q, want treatment", held, a)
		}
	}
}
