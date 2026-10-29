package hook

import (
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestMS5e_HookDrawsAtTheDefaultShare: the hook's context carries the
// session's draw at the default share, with nothing configured (card 2.5,
// D109 as amended; PRD PF-MS5): a session arm.Assign puts in the holdout at
// 1,000 basis points under the store's key gets Holdout true, and one it
// treats gets false.
func TestMS5e_HookDrawsAtTheDefaultShare(t *testing.T) {
	st := mustOpen(t)
	held, treated := armSessions(t, st)
	env := stEnv(steer.DisclosureVersion)
	if !drawnIn(st, env, held) || drawnIn(st, env, treated) {
		t.Errorf("Holdout is %v for %s (arm holdout) and %v for %s (arm treatment); want true and false",
			drawnIn(st, env, held), held, drawnIn(st, env, treated), treated)
	}
}

// TestMS5f_EnvOffSkipsTheDraw: EnvHoldout set to "off" skips the draw, so the
// session MS5e's search puts in the holdout reads Holdout false; "OFF" and
// "0" are ignored and the draw runs as before (card 2.5, D109 as amended).
func TestMS5f_EnvOffSkipsTheDraw(t *testing.T) {
	st := mustOpen(t)
	held, _ := armSessions(t, st)
	for _, tc := range []struct {
		value string
		want  bool
	}{{"off", false}, {"OFF", true}, {"0", true}} {
		env := stEnv(steer.DisclosureVersion)
		env.HoldoutOff = HoldoutOff(tc.value)
		if got := drawnIn(st, env, held); got != tc.want {
			t.Errorf("%s=%q: Holdout is %v for %s (arm holdout); want %v", EnvHoldout, tc.value, got, held, tc.want)
		}
	}
}

// armSessions finds one session of each arm at 1,000 basis points under the
// store's key, searching up to s256, not s64: the key is random per run, so
// at a tenth the chance of no holdout among 64 is 0.9^64, about one run in
// 830.
func armSessions(t *testing.T, st *store.Store) (held, treated string) {
	t.Helper()
	a, err := arm.New(st.Key())
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 256 && (held == "" || treated == ""); i++ {
		s := "s" + strconv.Itoa(i)
		got, err := a.Assign("steer", s, 1000)
		switch {
		case err != nil:
			t.Fatal(err)
		case got == arm.Holdout && held == "":
			held = s
		case got == arm.Treatment && treated == "":
			treated = s
		}
	}
	if held == "" || treated == "" {
		t.Fatalf("s1 to s256 found holdout %q and treatment %q; want one of each", held, treated)
	}
	return held, treated
}

// drawnIn is the Holdout contextOf gives session's call under env.
func drawnIn(st *store.Store, env SteerEnv, session string) bool {
	p := &Post{st: st, sessionID: session, promptID: "p1", permissionMode: "default", event: FailureEvent, cwd: "/tmp/project"}
	return p.contextOf(env, verify.SteerView{ProjectRoot: "/repo"}).Holdout
}
