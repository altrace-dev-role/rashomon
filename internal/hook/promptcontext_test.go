package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestPF1m_PromptContextDrawsAsThePostDoes: the preflight's Route context,
// made outside a Post for recap's UserPromptSubmit catch-up, carries the new
// prompt's id, the marker's consent and the same holdout draw a post of the
// same session gets, so a held-out session's preflight goes to neither side,
// like its other notes. The draw is on here by EnvHoldout's "on".
func TestPF1m_PromptContextDrawsAsThePostDoes(t *testing.T) {
	st := mustOpen(t)
	held, treated := armSessions(t, st)
	env := stEnv(steer.DisclosureVersion)
	env.HoldoutOn = true

	ctx := PromptContext(st, nil, held, "prompt-2", "/repo", "default", env)
	if ctx.Event != steer.EventUserPromptSubmit || ctx.Store != st || ctx.SessionID != held || ctx.Project != "/repo" {
		t.Errorf("context = event %q, session %q, project %q; want UserPromptSubmit, %q, /repo", ctx.Event, ctx.SessionID, ctx.Project, held)
	}
	if ctx.PromptID == nil || *ctx.PromptID != "prompt-2" {
		t.Errorf("PromptID = %v, want prompt-2: the new prompt's id, which the budget counts against", ctx.PromptID)
	}
	if !ctx.Consent || ctx.NowUnixMS != env.Now.UnixMilli() || ctx.Arm != steer.ArmLive || ctx.Attendance != steer.AttendanceAttended {
		t.Errorf("context = consent %v, now %d, arm %q, attendance %q", ctx.Consent, ctx.NowUnixMS, ctx.Arm, ctx.Attendance)
	}
	if ctx.Holdout != drawnIn(st, env, held) || !ctx.Holdout {
		t.Errorf("Holdout = %v for %s, want the post's draw, true", ctx.Holdout, held)
	}
	if PromptContext(st, nil, treated, "prompt-2", "/repo", "default", env).Holdout {
		t.Errorf("Holdout = true for %s (arm treatment), want false", treated)
	}
	if got := PromptContext(st, nil, held, "", "/repo", "default", env).PromptID; got != nil {
		t.Errorf("PromptID = %q with no prompt id, want nil", *got)
	}
	if PromptContext(st, nil, held, "prompt-2", "/repo", "default", stEnv(steer.DisclosureVersion-1)).Consent {
		t.Error("Consent = true under an older disclosure, want false")
	}
	if PromptContext(st, nil, held, "prompt-2", "/repo", "dontAsk", env).Attendance != steer.AttendanceUnattended {
		t.Error("dontAsk reads attended, want unattended")
	}
}
