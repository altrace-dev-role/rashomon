package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestPF1z_AttendanceFollowsTheSurface: on the prompt path (D207) CI alone
// does not make the call unattended, an SDK or cloud surface does, and dontAsk
// still does; each against its control. A PostToolUse call keeps ED-9: CI
// makes it unattended whatever the surface, so the nudge it raises is
// suppressed no_mirror.
func TestPF1z_AttendanceFollowsTheSurface(t *testing.T) {
	st := mustOpen(t)
	for _, c := range []struct {
		name    string
		surface string
		ci      bool
		mode    string
		want    steer.Attendance
	}{
		{"CI on an interactive surface", "cli", true, "default", steer.AttendanceAttended},
		{"control: no CI", "cli", false, "default", steer.AttendanceAttended},
		{"the sdk surface", "sdk", false, "default", steer.AttendanceUnattended},
		{"the cloud surface", "cloud", false, "default", steer.AttendanceUnattended},
		{"dontAsk on an interactive surface", "cli", false, "dontAsk", steer.AttendanceUnattended},
	} {
		env := stEnv(steer.DisclosureVersion)
		env.Surface, env.CI = c.surface, c.ci
		if got := PromptContext(st, nil, "s", "p", "/repo", c.mode, env).Attendance; got != c.want {
			t.Errorf("prompt path, %s: attendance %q, want %q", c.name, got, c.want)
		}
	}
	for _, surface := range []string{"cli", "sdk"} {
		env := stEnv(steer.DisclosureVersion)
		env.Surface, env.CI = surface, true
		p := &Post{st: st, sessionID: "s", promptID: "p", cwd: "/repo", event: string(steer.EventPostToolUse)}
		if got := p.contextOf(env, verify.SteerView{}).Attendance; got != steer.AttendanceUnattended {
			t.Errorf("PostToolUse under CI on %s: attendance %q, want unattended (ED-9)", surface, got)
		}
		env.CI = false
		if got := p.contextOf(env, verify.SteerView{}).Attendance; got != steer.AttendanceAttended {
			t.Errorf("PostToolUse without CI on %s: attendance %q, want attended (control)", surface, got)
		}
	}
}

// TestPF1w_PromptContextAtFilesTheLineUnderTheProject: the line's project is
// the one the caller passes (the record scope's), while the steer switches
// read the cwd's project.
func TestPF1w_PromptContextAtFilesTheLineUnderTheProject(t *testing.T) {
	st := mustOpen(t)
	env := stEnv(steer.DisclosureVersion)
	if got := PromptContextAt(st, nil, "s", "p", "/repo", "", "default", env).Project; got != "" {
		t.Errorf("project %q at v4, want \"\"", got)
	}
	if got := PromptContextAt(st, nil, "s", "p", "/repo", "/key", "default", env).Project; got != "/key" {
		t.Errorf("project %q at v5, want the key", got)
	}
}
