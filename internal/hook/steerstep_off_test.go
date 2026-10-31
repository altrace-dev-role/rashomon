package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// stcConfig puts raw in the store's config.json.
func stcConfig(t *testing.T, st *store.Store, raw string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(st.Root(), store.FileConfig), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// stcRed lands the regression under the config raw (card STEER-CLI1) and
// returns what the step printed and the steer log's lines.
func stcRed(t *testing.T, raw string) ([]byte, []steer.Line) {
	t.Helper()
	f := newST(t)
	if raw != "" {
		stcConfig(t, f.st, raw)
	}
	out := f.red("toolu_r").Steer(stEnv(steer.DisclosureVersion))
	return out, steerLines(t, f.st)
}

// stcClamped reports the step printed the note as a user-only flag, nothing
// for the agent, and every line it logged is a steer_off clamp: delivered at
// level flag, delivery flag_only. At least one line.
func stcClamped(t *testing.T, out []byte, lines []steer.Line) bool {
	t.Helper()
	return stcClampedBy(t, out, lines, steer.SuppressedBySteerOff)
}

// stcClampedBy is stcClamped for the clamp reason by.
func stcClampedBy(t *testing.T, out []byte, lines []steer.Line, by steer.SuppressedBy) bool {
	t.Helper()
	if !printsFlag(t, out) || len(lines) == 0 {
		return false
	}
	for _, l := range lines {
		if l.State != steer.LineStateDelivered || l.Level != "flag" || l.SuppressedBy == nil || *l.SuppressedBy != by ||
			l.Delivery == nil || *l.Delivery != steer.DeliveryFlagOnly {
			return false
		}
	}
	return true
}

// TestH145_SteerOffUserPrintsNothingAndLogsSteerOff (card STEER-CLI1): the
// post with the user's steer switch off carries no additionalContext, shows
// the user the flag the note would have been, and its line says suppressed_by
// steer_off, delivery flag_only. Without the switch the same call is not that
// flag (the premise), so the clamp is the switch's.
func TestH145_SteerOffUserClampsToAFlagAndLogsSteerOff(t *testing.T) {
	if out, lines := stcRed(t, ""); stcClamped(t, out, lines) {
		t.Fatalf("premise: the regression is a steer_off flag with the switch unset")
	}
	out, lines := stcRed(t, `{"steer_user":"off"}`)
	if !stcClamped(t, out, lines) {
		t.Errorf("steer_user off: printed %q, lines %+v", out, lines)
	}
}

// TestSTC1b_ProjectKeyIsThatProjectsOnly: steer_projects names a project by
// baseline.Key of the call's cwd, and only that project is silenced; another
// project's key, and an explicit steer_user "on", silence nothing.
func TestSTC1b_ProjectKeyIsThatProjectsOnly(t *testing.T) {
	here := baseline.Key("/tmp/project")
	out, lines := stcRed(t, `{"steer_projects":{"`+here+`":"off"}}`)
	if !stcClamped(t, out, lines) {
		t.Errorf("this project off: printed %q, lines %+v", out, lines)
	}
	out, lines = stcRed(t, `{"steer_projects":{"/somewhere/else":"off"},"steer_user":"on"}`)
	if stcClamped(t, out, lines) {
		t.Errorf("another project off: printed %q, lines %+v", out, lines)
	}
}

// TestSTC1g_AnUnreadableConfigClamps: a config.json that does not parse is
// not read as the notes being on: a switch that cannot be read is not one
// the hook may override, so nothing reaches the agent (the clamp). Its own
// reason, config_unreadable, tells the person to fix the file; steer_off
// would say to run `steer on`, which refuses it.
func TestSTC1g_AnUnreadableConfigClamps(t *testing.T) {
	out, lines := stcRed(t, `{"steer_user":`)
	if !stcClampedBy(t, out, lines, steer.SuppressedByConfigUnreadable) {
		t.Errorf("unreadable config: printed %q, lines %+v", out, lines)
	}
	out, lines = stcRed(t, `{"steer_user":"off"}`)
	if !stcClamped(t, out, lines) {
		t.Errorf("steer_user off: printed %q, lines %+v", out, lines)
	}
}

// TestSTC1n_AValueThatIsNotOnClampsAtTheHook: "Off" and "disabled" are not
// "on", so the hook clamps on them in either scope.
func TestSTC1n_AValueThatIsNotOnClampsAtTheHook(t *testing.T) {
	here := baseline.Key("/tmp/project")
	for _, raw := range []string{`{"steer_user":"Off"}`, `{"steer_user":"disabled"}`, `{"steer_projects":{"` + here + `":"Off"}}`} {
		out, lines := stcRed(t, raw)
		if !stcClamped(t, out, lines) {
			t.Errorf("%s: printed %q, lines %+v", raw, out, lines)
		}
	}
}

// TestSTC1h_PromptContextCarriesTheSwitch: the preflight's context is a
// post's, so the user's switch and the project's reach it too.
func TestSTC1h_PromptContextCarriesTheSwitch(t *testing.T) {
	st := mustOpen(t)
	here := baseline.Key("/tmp/project")
	env := stEnv(steer.DisclosureVersion)
	if PromptContext(st, nil, "s", "p", "/tmp/project", "default", env).SteerOff {
		t.Fatal("premise: SteerOff with no config")
	}
	stcConfig(t, st, `{"steer_projects":{"`+here+`":"off"}}`)
	if !PromptContext(st, nil, "s", "p", "/tmp/project", "default", env).SteerOff {
		t.Error("the project's key did not reach the prompt's context")
	}
	stcConfig(t, st, `{"steer_user":"off"}`)
	if !PromptContext(st, nil, "s", "p", "/elsewhere", "default", env).SteerOff {
		t.Error("the user's key did not reach the prompt's context")
	}
}
