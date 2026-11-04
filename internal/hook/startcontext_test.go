package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestCSL1_StartContextCarriesItsSourceToTheChannelRow: the continuity
// briefing's Route context at SessionStart carries the start's source, the
// second half of that event's channel row, so a compact start on 2.1.316
// reads the row P-TRU-1 opened and a startup start the row it did not. It
// has no prompt. Break: drop the source, or build it as a post's context
// for another event, and this fails.
func TestCSL1_StartContextCarriesItsSourceToTheChannelRow(t *testing.T) {
	st := mustOpen(t)
	env := promptEnv(harness.ClaudeCode, harness.SurfaceCLI, false)
	env.AIAgent = "claude-code_2-1-316_harness"
	ctx := StartContextAt(st, nil, "s", "/repo", "", "default", "compact", env)
	if ctx.Event != steer.EventSessionStart || ctx.SessionID != "s" || ctx.PromptID != nil || ctx.Client.StartSource != "compact" {
		t.Errorf("context = event %q, session %q, prompt %v, source %q; want SessionStart, s, none, compact",
			ctx.Event, ctx.SessionID, ctx.PromptID, ctx.Client.StartSource)
	}
	if !steer.ContextAllowed(string(steer.EventSessionStart), ctx.Client) {
		t.Errorf("SessionStart(compact) on %s: the channel reads closed, want the row P-TRU-1 opened", env.AIAgent)
	}
	startup := StartContextAt(st, nil, "s", "/repo", "", "default", "startup", env)
	if steer.ContextAllowed(string(steer.EventSessionStart), startup.Client) {
		t.Errorf("SessionStart(startup) on %s: the channel reads open, want closed (P-TRU-1 failed it)", env.AIAgent)
	}
	if got := StartContextAt(st, nil, "s", "/repo", "/key", "default", "compact", env).Project; got != "/key" {
		t.Errorf("project %q, want the one passed", got)
	}
}

// TestCSL1_StartAttendsAsThePromptDoes: the briefing quotes what a person
// typed back to the model, context for it and not a mirror for a watcher, so
// SessionStart's attendance is the prompt path's (attendanceOf): CI alone
// does not make it unattended, a surface Caps says no person is at does, and
// dontAsk does. Break: leave SessionStart on the post's rule, and the CI case
// fails.
func TestCSL1_StartAttendsAsThePromptDoes(t *testing.T) {
	st := mustOpen(t)
	for _, c := range []struct {
		name    string
		surface harness.Surface
		ci      bool
		mode    string
		want    steer.Attendance
	}{
		{"CI on the CLI", harness.SurfaceCLI, true, "default", steer.AttendanceAttended},
		{"control: no CI", harness.SurfaceCLI, false, "default", steer.AttendanceAttended},
		{"the GitHub Action: the cloud surface under CI", harness.SurfaceCloud, true, "default", steer.AttendanceAttended},
		{"the sdk surface", harness.SurfaceSDK, false, "default", steer.AttendanceUnattended},
		{"dontAsk", harness.SurfaceCLI, false, "dontAsk", steer.AttendanceUnattended},
	} {
		env := promptEnv(harness.ClaudeCode, c.surface, c.ci)
		if got := StartContextAt(st, nil, "s", "/repo", "", c.mode, "compact", env).Attendance; got != c.want {
			t.Errorf("%s: attendance %q, want %q", c.name, got, c.want)
		}
	}
}
