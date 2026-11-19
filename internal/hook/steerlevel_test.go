package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestSteerLevel_ReachesTheContext: the port's Level is the context's
// LevelOverride, and the zero Pair when the port names none. Break: drop
// it from contextOf, and a lab run's override never reaches the router.
func TestSteerLevel_ReachesTheContext(t *testing.T) {
	st := mustOpen(t)
	for _, want := range []steer.Pair{{}, {Pattern: pattern.RegressionLocation, Kind: "k"}} {
		env := stEnv(steer.DisclosureVersion)
		env.Level = want
		p := &Post{st: st, sessionID: "s1", promptID: "p1", permissionMode: "default", event: FailureEvent, cwd: "/tmp/project"}
		if got := p.contextOf(env, verify.SteerView{ProjectRoot: "/repo"}).LevelOverride; got != want {
			t.Errorf("contextOf under port level %+v names %+v", want, got)
		}
	}
}
