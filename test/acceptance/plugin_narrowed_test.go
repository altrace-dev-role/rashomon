package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRubricHLD1_NarrowedMatcherPluginHooksReadAbsent: a plugin whose
// PreToolUse matcher has been narrowed to "Bash" is not the entry watch
// installs, and the coverage record says so. Before the plugin check put
// each group through coverageIntact, this read present_plugin, and a
// session under a "Bash" matcher rendered verified while every Edit, Write
// and MCP call went unrecorded.
func TestRubricHLD1_NarrowedMatcherPluginHooksReadAbsent(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	narrowPluginPreToolUse(t, filepath.Join(fx.root, "hooks", "hooks.json"), "matcher", "Bash")

	if res := e.runBin(fx.bin, e.sessionPayload("start", testSession), "probe", "start"); res.exitCode != 0 {
		t.Fatalf("probe start via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
		t.Fatalf("hook via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	assertCallCoverage(t, e, testSession, "unverified", "hook_entry_absent")
	recs := e.coverage(testSession, "call")
	if got := recs[len(recs)-1].str("hook_entry"); got != "absent" {
		t.Errorf("call coverage hook_entry is %q under a plugin with PreToolUse matcher=Bash, want absent", got)
	}
}

// TestRubricHLD1_RetimedPluginHookStillCoversEveryCall: a plugin whose
// PreToolUse timeout has been changed to 600 still fires our command on
// every call, and the coverage record reads present_plugin, not absent. A
// different timeout changes how long Claude Code waits for the hook, not
// whether it runs, so it must not cost coverage the way a narrowed matcher
// does; before coverageIntact existed, this read absent beside a session
// that had in fact recorded cleanly.
func TestRubricHLD1_RetimedPluginHookStillCoversEveryCall(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	narrowPluginPreToolUse(t, filepath.Join(fx.root, "hooks", "hooks.json"), "timeout", 600)

	if res := e.runBin(fx.bin, e.sessionPayload("start", testSession), "probe", "start"); res.exitCode != 0 {
		t.Fatalf("probe start via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
		t.Fatalf("hook via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	assertCallCoverage(t, e, testSession, "verified", "")
	recs := e.coverage(testSession, "call")
	if got := recs[len(recs)-1].str("hook_entry"); got != "present_plugin" {
		t.Errorf("call coverage hook_entry is %q under a plugin with PreToolUse timeout=600, want present_plugin", got)
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
