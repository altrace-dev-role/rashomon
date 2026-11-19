package main

import (
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestSteerLevel_HonouredOnlyUnderALabArm: RASHOMON_STEER_LEVEL is read
// only where operatorArm names an arm. On a store with no lab marker it
// reads as unset under either arm, as on a lab store with no arm; on a lab
// store both arms read it, so the off arm decides the pair as the steer arm
// delivers it. The port carries the value as SteerLevel reads it. Break:
// read it on any store, or under the steer arm alone.
func TestSteerLevel_HonouredOnlyUnderALabArm(t *testing.T) {
	v := string(pattern.RegressionLocation) + "/"
	t.Setenv(hook.EnvSteerLevel, v)
	plain, lab := t.TempDir(), labRoot(t)
	for _, tc := range []struct {
		name, root, arm, want string
	}{
		{"plain", plain, "steer", ""}, {"plain", plain, "off", ""},
		{"lab", lab, "", ""}, {"lab", lab, "live", ""},
		{"lab", lab, "off", v}, {"lab", lab, "steer", v},
	} {
		t.Setenv(hook.EnvSteerArm, tc.arm)
		if got := operatorLevel(operatorArm(tc.root)); got != tc.want {
			t.Errorf("%s store, arm %q: operatorLevel = %q, want %q", tc.name, tc.arm, got, tc.want)
		}
		if got, want := steerEnv(harness.Identity{}, time.Unix(0, 0), tc.root).Level, hook.SteerLevel(tc.want); got != want {
			t.Errorf("%s store, arm %q: steerEnv carries level %+v, want %+v", tc.name, tc.arm, got, want)
		}
	}
}

// TestSteerLevel_UnknownPairIgnored: under the steer arm on a lab store, a
// value that is not one held pair reads as no override, with no error: no
// "/", a pair no row holds, an unknown pattern, two pairs, and a spelling
// off by case or by a space. Break: let SteerLevel return a pair it parsed
// without asking whether it is held.
func TestSteerLevel_UnknownPairIgnored(t *testing.T) {
	root := labRoot(t)
	t.Setenv(hook.EnvSteerArm, "steer")
	r := string(pattern.RegressionLocation)
	for _, v := range []string{
		"", "/", r, r + "/", r + "/other", "no-such-pattern/x",
		r + "/," + string(pattern.StaleEvidence) + "/", strings.ToUpper(r) + "/", " " + r + "/",
	} {
		if got := hook.SteerLevel(v); got != (steer.Pair{}) {
			t.Errorf("SteerLevel(%q) = %+v, want none", v, got)
		}
		t.Setenv(hook.EnvSteerLevel, v)
		if got := steerEnv(harness.Identity{}, time.Unix(0, 0), root).Level; got != (steer.Pair{}) {
			t.Errorf("%s=%q gives level %+v, want none", hook.EnvSteerLevel, v, got)
		}
	}
}
