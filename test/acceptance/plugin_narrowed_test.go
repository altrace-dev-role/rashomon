package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRubricHLD1_NarrowedPluginHooksReadAbsent: a plugin whose hooks.json
// has been narrowed -- PreToolUse matched to "Bash" only, or given a timeout
// of 600 -- is not the entry watch installs, and the coverage record says so.
// Before the plugin check put each group through intact(), both read
// present_plugin, and a session under a "Bash" matcher rendered verified
// while every Edit, Write and MCP call went unrecorded.
func TestRubricHLD1_NarrowedPluginHooksReadAbsent(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value any
	}{
		{"matcher Bash", "matcher", "Bash"},
		{"timeout 600", "timeout", 600},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			fx := newPluginFixture(t, "rashomon@test")
			fx.installed(t, e)
			fx.enable(t, e, true)
			narrowPluginPreToolUse(t, filepath.Join(fx.root, "hooks", "hooks.json"), c.field, c.value)

			if res := e.runBin(fx.bin, e.sessionPayload("start", testSession), "probe", "start"); res.exitCode != 0 {
				t.Fatalf("probe start via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
				t.Fatalf("hook via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			assertCallCoverage(t, e, testSession, "unverified", "hook_entry_absent")
			recs := e.coverage(testSession, "call")
			if got := recs[len(recs)-1].str("hook_entry"); got != "absent" {
				t.Errorf("call coverage hook_entry is %q under a plugin with PreToolUse %s=%v, want absent",
					got, c.field, c.value)
			}
		})
	}
}

// narrowPluginPreToolUse rewrites one field of the PreToolUse group in a
// plugin's hooks.json: matcher on the group, timeout on its hook.
func narrowPluginPreToolUse(t *testing.T, path, field string, value any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	group := doc["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if field == "timeout" {
		group["hooks"].([]any)[0].(map[string]any)[field] = value
	} else {
		group[field] = value
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, path, string(out))
}
