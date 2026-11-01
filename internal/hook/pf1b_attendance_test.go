package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// promptEnv is the steer port as cmd builds it for a call from harness id on
// surface: the identity the host guard detected and the in-band mirror as
// harness.Caps reads that pair (cmd/rashomon/steerenv.go), so each case below
// reaches attendance through the same table the binary does.
func promptEnv(id harness.ID, surface harness.Surface, ci bool) SteerEnv {
	env := stEnv(steer.DisclosureVersion)
	env.HarnessID, env.Surface, env.CI = string(id), string(surface), ci
	env.MirrorInBand = harness.Caps(id, surface)[harness.CapMirrorInBand] == harness.Yes
	return env
}

// TestPF1z_AttendanceFollowsTheSurface: on the prompt path (D207) CI alone
// does not make the call unattended; whether a person is at the prompt does
// (harness.Caps, PREFLIGHT-CLOUD1), and dontAsk still does; each against its
// control. Claude Code on the web and the GitHub Action are the cloud
// surface (remote*, claude-code-github-action), attended, so their note is
// not withheld as no_mirror; an SDK program has no one at a prompt; a
// presumed Claude Code with no entrypoint stays attended, as before; a
// harness Detect could not name gets nothing from Caps, so no person is
// claimed and its note is withheld. A PostToolUse call keeps ED-9: CI makes
// it unattended whatever the surface, so the nudge it raises is suppressed
// no_mirror.
func TestPF1z_AttendanceFollowsTheSurface(t *testing.T) {
	st := mustOpen(t)
	for _, c := range []struct {
		name    string
		id      harness.ID
		surface harness.Surface
		ci      bool
		mode    string
		want    steer.Attendance
	}{
		{"CI on an interactive surface", harness.ClaudeCode, harness.SurfaceCLI, true, "default", steer.AttendanceAttended},
		{"control: no CI", harness.ClaudeCode, harness.SurfaceCLI, false, "default", steer.AttendanceAttended},
		{"the IDE surface", harness.ClaudeCode, harness.SurfaceIDE, false, "default", steer.AttendanceAttended},
		{"the desktop surface", harness.ClaudeCode, harness.SurfaceDesktop, false, "default", steer.AttendanceAttended},
		{"the cloud surface: the web session", harness.ClaudeCode, harness.SurfaceCloud, false, "default", steer.AttendanceAttended},
		{"the cloud surface under CI: the GitHub Action", harness.ClaudeCode, harness.SurfaceCloud, true, "default", steer.AttendanceAttended},
		{"the sdk surface", harness.ClaudeCode, harness.SurfaceSDK, false, "default", steer.AttendanceUnattended},
		{"no entrypoint: a presumed Claude Code", harness.ClaudeCodePresumed, harness.SurfaceUnknown, false, "default", steer.AttendanceAttended},
		{"an unknown harness: Caps claims nothing", harness.Unknown, harness.SurfaceUnknown, false, "default", steer.AttendanceUnattended},
		{"dontAsk on an interactive surface", harness.ClaudeCode, harness.SurfaceCLI, false, "dontAsk", steer.AttendanceUnattended},
		{"dontAsk on the cloud surface", harness.ClaudeCode, harness.SurfaceCloud, false, "dontAsk", steer.AttendanceUnattended},
	} {
		env := promptEnv(c.id, c.surface, c.ci)
		if got := PromptContext(st, nil, "s", "p", "/repo", c.mode, env).Attendance; got != c.want {
			t.Errorf("prompt path, %s: attendance %q, want %q", c.name, got, c.want)
		}
	}
	for _, surface := range []string{"cli", "sdk", "cloud"} {
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
