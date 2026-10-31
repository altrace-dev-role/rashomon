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

// stcSilenced reports the step printed nothing and every line it logged is a
// steer_off suppression, at least one.
func stcSilenced(out []byte, lines []steer.Line) bool {
	if len(out) != 0 || len(lines) == 0 {
		return false
	}
	for _, l := range lines {
		if l.State != steer.LineStateSuppressed || l.SuppressedBy == nil || *l.SuppressedBy != steer.SuppressedBySteerOff {
			return false
		}
	}
	return true
}

// TestH145_SteerOffUserPrintsNothingAndLogsSteerOff (card STEER-CLI1): the
// post with the user's steer switch off carries no additionalContext and no
// systemMessage, and its line says suppressed steer_off. Without the switch
// the same call prints (the premise), so the silence is the switch's.
func TestH145_SteerOffUserPrintsNothingAndLogsSteerOff(t *testing.T) {
	if out, _ := stcRed(t, ""); len(out) == 0 {
		t.Fatalf("premise: the regression prints nothing with the switch unset")
	}
	out, lines := stcRed(t, `{"steer_user":"off"}`)
	if !stcSilenced(out, lines) {
		t.Errorf("steer_user off: printed %q, lines %+v", out, lines)
	}
}

// TestSTC1b_ProjectKeyIsThatProjectsOnly: steer_projects names a project by
// baseline.Key of the call's cwd, and only that project is silenced; another
// project's key, and an explicit steer_user "on", silence nothing.
func TestSTC1b_ProjectKeyIsThatProjectsOnly(t *testing.T) {
	here := baseline.Key("/tmp/project")
	out, lines := stcRed(t, `{"steer_projects":{"`+here+`":"off"}}`)
	if !stcSilenced(out, lines) {
		t.Errorf("this project off: printed %q, lines %+v", out, lines)
	}
	out, lines = stcRed(t, `{"steer_projects":{"/somewhere/else":"off"},"steer_user":"on"}`)
	if stcSilenced(out, lines) || len(out) == 0 {
		t.Errorf("another project off: printed %q, lines %+v", out, lines)
	}
}

// TestSTC1g_AnUnreadableConfigIsQuiet: a config.json that does not parse is
// not read as the notes being on: a switch that cannot be read is not one
// the hook may override, and the notes are model-visible.
func TestSTC1g_AnUnreadableConfigIsQuiet(t *testing.T) {
	out, lines := stcRed(t, `{"steer_user":`)
	if !stcSilenced(out, lines) {
		t.Errorf("unreadable config: printed %q, lines %+v", out, lines)
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
