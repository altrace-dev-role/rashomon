package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBR1i_TheBriefPointsAtTheCommandItsOriginHas: probe start's verify line
// follows the recap pointer's rule (TestH91_TheReportPointerNamesACommandTheOriginHas)
// with the plugin's renamed command (D175). The plugin's entry names
// /rashomon:report last; a settings install, the CLI's report of the
// payload's session, including a settings install made from the plugin's
// binary, which sits in a plugin directory and carries the --install marker.
//
// Break: decide the origin from where the executable lives alone and the
// middle case names the plugin's command; hard-code either answer and one of
// the other two cases does.
func TestBR1i_TheBriefPointsAtTheCommandItsOriginHas(t *testing.T) {
	const (
		cliVerify    = "rashomon report --session " + testSession
		pluginVerify = "/rashomon:report last"
	)
	verify := func(t *testing.T, e *env, bin string, marker ...string) string {
		t.Helper()
		res := e.runBin(bin, e.sessionPayload("start", testSession), append([]string{"probe", "start"}, marker...)...)
		if res.exitCode != 0 {
			t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		var out struct {
			SystemMessage string `json:"systemMessage"`
		}
		if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
			t.Fatalf("probe start stdout is not JSON: %v\n%s", err, res.stdout)
		}
		_, line, ok := strings.Cut(out.SystemMessage, "\n  verify: ")
		if !ok {
			t.Fatalf("no verify line in %q", out.SystemMessage)
		}
		return line
	}

	t.Run("settings install", func(t *testing.T) {
		e := newEnv(t)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if line := verify(t, e, rashomonBin, e.installArgs()...); line != cliVerify {
			t.Errorf("verify %q, want %q, the only command a settings install has", line, cliVerify)
		}
	})

	t.Run("settings install made from the plugin's binary", func(t *testing.T) {
		e := newEnv(t)
		fx := newPluginFixture(t, "rashomon@test")
		if res := e.runBin(fx.bin, "", "watch", "--yes"); res.exitCode != 0 {
			t.Fatalf("watch via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if line := verify(t, e, fx.bin, e.installArgs()...); line != cliVerify {
			t.Errorf("verify %q, want %q: the settings entry ran this, and no plugin is loaded", line, cliVerify)
		}
	})

	t.Run("plugin", func(t *testing.T) {
		e := newEnv(t)
		fx := newPluginFixture(t, "rashomon@test")
		fx.installed(t, e)
		fx.enable(t, e, true)
		if line := verify(t, e, fx.bin); line != pluginVerify {
			t.Errorf("verify %q, want %q, the plugin's own command", line, pluginVerify)
		}
	})
}
