package hook

import (
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestMS5e_HookDrawsAtTheDefaultShare: under EnvHoldout "on", with no spec,
// the hook's context carries the session's draw at the default share (card
// 2.5; PRD PF-MS5): a session arm.Assign puts in the holdout at 1,000 basis
// points under the store's key gets Holdout true, and one it treats gets
// false.
func TestMS5e_HookDrawsAtTheDefaultShare(t *testing.T) {
	st := mustOpen(t)
	held, treated := armSessions(t, st)
	env := stEnv(steer.DisclosureVersion)
	env.HoldoutOn = HoldoutOn("on")
	if !drawnIn(st, env, held) || drawnIn(st, env, treated) {
		t.Errorf("Holdout is %v for %s (arm holdout) and %v for %s (arm treatment); want true and false",
			drawnIn(st, env, held), held, drawnIn(st, env, treated), treated)
	}
}

// TestVG1e_NoSessionIsHeldOutByDefault: with nothing configured the share is
// 0 for the alpha, so the session arm holds at 1,000 basis points reads
// Holdout false; EnvHoldout turns it on for "on" exactly, and "ON" and "1"
// are ignored (card VERSION-GATE1).
func TestVG1e_NoSessionIsHeldOutByDefault(t *testing.T) {
	st := mustOpen(t)
	held, _ := armSessions(t, st)
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", false}, {"on", true}, {"ON", false}, {"1", false}, {"off", false}} {
		env := stEnv(steer.DisclosureVersion)
		env.HoldoutOn, env.HoldoutOff = HoldoutOn(tc.value), HoldoutOff(tc.value)
		if got := drawnIn(st, env, held); got != tc.want {
			t.Errorf("%s=%q: Holdout is %v for %s (arm holdout); want %v", EnvHoldout, tc.value, got, held, tc.want)
		}
	}
}

// TestMS5f_EnvOffSkipsTheDraw: EnvHoldout set to "off" skips the draw, so the
// session MS5e's search puts in the holdout reads Holdout false where the
// draw is on; "OFF" and "0" are ignored and the draw runs as before (card
// 2.5, D109 as amended). The draw is turned on here by a spec's holdout
// object, which "off" overrides as well.
func TestMS5f_EnvOffSkipsTheDraw(t *testing.T) {
	st := mustOpen(t)
	held, _ := armSessions(t, st)
	for _, tc := range []struct {
		value string
		want  bool
	}{{"off", false}, {"OFF", true}, {"0", true}} {
		env := stEnv(steer.DisclosureVersion)
		env.HoldoutOff = HoldoutOff(tc.value)
		if got := drawnWithSpec(t, st, env, `{"rashomon_spec": 1, "holdout": {}}`, true, held); got != tc.want {
			t.Errorf("%s=%q: Holdout is %v for %s (arm holdout); want %v", EnvHoldout, tc.value, got, held, tc.want)
		}
	}
}

// TestVG1f_TheSpecsHoldoutObjectTurnsItOn: a trusted spec's holdout object
// turns the draw on at its share with nothing in the environment; an
// untrusted spec's is not read, so a repository cannot opt its user into a
// session whose notes reach neither side, and the share stays as the
// environment says; a spec without one leaves it as the environment says:
// off by default, on at 1,000 under "on" (card VERSION-GATE1).
func TestVG1f_TheSpecsHoldoutObjectTurnsItOn(t *testing.T) {
	st := mustOpen(t)
	held, _ := armSessions(t, st)
	on := stEnv(steer.DisclosureVersion)
	on.HoldoutOn = true
	for _, tc := range []struct {
		name    string
		env     SteerEnv
		doc     string
		trusted bool
		want    bool
	}{
		{"a trusted empty holdout object", stEnv(steer.DisclosureVersion), `{"rashomon_spec": 1, "holdout": {}}`, true, true},
		{"an untrusted empty holdout object", stEnv(steer.DisclosureVersion), `{"rashomon_spec": 1, "holdout": {}}`, false, false},
		{"an untrusted object at 2000", stEnv(steer.DisclosureVersion), `{"rashomon_spec": 1, "holdout": {"rate_bp": 2000}}`, false, false},
		{"an untrusted object off, env on", on, `{"rashomon_spec": 1, "holdout": {"off": true}}`, false, true},
		{"a spec with no holdout object", stEnv(steer.DisclosureVersion), `{"rashomon_spec": 1}`, true, false},
		{"a spec with no holdout object, env on", on, `{"rashomon_spec": 1}`, true, true},
		{"the trusted object off, env on", on, `{"rashomon_spec": 1, "holdout": {"off": true}}`, true, false},
	} {
		if got := drawnWithSpec(t, st, tc.env, tc.doc, tc.trusted, held); got != tc.want {
			t.Errorf("%s: Holdout is %v for %s (arm holdout at 1,000); want %v", tc.name, got, held, tc.want)
		}
	}
}

// drawnWithSpec is the Holdout contextOf gives session's call under env with
// doc as the one discovered spec, bound under st's root when trusted.
func drawnWithSpec(t *testing.T, st *store.Store, env SteerEnv, doc string, trusted bool, session string) bool {
	t.Helper()
	s := newSpecTree(t)
	s.mkdir(".git")
	path := s.put(s.base, doc)
	if trusted {
		_, digest, err := spec.Parse([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		writeBindings(t, st.Root(), spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: digest, BoundAtMS: 1}, false)
	}
	p := &Post{st: st, sessionID: session, promptID: "p1", permissionMode: "default", event: FailureEvent, cwd: s.base}
	if p.sp = DiscoverSpec(p.cwd, "", "", NoRun(st.Root())); p.sp == nil || len(p.sp.Sources) != 1 {
		t.Fatalf("DiscoverSpec: %+v; want the one spec", p.sp)
	}
	if want := map[bool]spec.Trust{true: spec.Trusted, false: spec.Untrusted}[trusted]; p.sp.Sources[0].Trust != want {
		t.Fatalf("premise: the spec reads %s, want %s", p.sp.Sources[0].Trust, want)
	}
	return p.contextOf(env, verify.SteerView{ProjectRoot: s.base}).Holdout
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
