package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests below cover a plugin whose every hooks.json group has been hand
// edited -- each hook's timeout changed. No group is the entry watch installs,
// so coverage reads hook_entry absent for it (TestRubricHLD1_NarrowedPlugin
// HooksReadAbsent). The plugin still fires on every one of those events, so
// the questions "would a settings install record twice beside it" and "will
// anything record a tool call" are about whether it declares our commands at
// all, not whether it declares them intact.

// TestRubricHLD1_WatchRefusesUnderAHandEditedPlugin: watch refuses beside an
// enabled plugin whose groups are all hand edited, as it does beside an
// untouched one. Under the intact-only list the refusal saw no events and
// installed settings entries next to a plugin that still records every call.
func TestRubricHLD1_WatchRefusesUnderAHandEditedPlugin(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	retimeEveryPluginHook(t, filepath.Join(fx.root, "hooks", "hooks.json"), 30)
	before := e.settingsBytes()

	res := e.watch()
	if res.exitCode == 0 {
		t.Fatalf("watch installed beside the enabled %s plugin whose hand-edited entries still fire", fx.key)
	}
	if !strings.Contains(res.stderr, fx.key) || !strings.Contains(res.stderr, "PreToolUse") {
		t.Errorf("refusal does not name the plugin and the events it fires on: %q", res.stderr)
	}
	if !bytes.Equal(before, e.settingsBytes()) {
		t.Errorf("a refusing watch still wrote the settings file")
	}
}

// TestRubricHLD1_StatusNamesTheOverlapUnderAHandEditedPlugin: a settings
// install beside an enabled, hand-edited plugin records every call twice,
// and status says so. The plugin's per-event lines say present too, because
// the plugin does fire on every call -- "absent" there read as a
// contradiction of the overlap line below it, which is what Live() (reading
// Declared, the fires-at-all list) already reported.
func TestRubricHLD1_StatusNamesTheOverlapUnderAHandEditedPlugin(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	retimeEveryPluginHook(t, filepath.Join(fx.root, "hooks", "hooks.json"), 30)

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	want := "present (hand edited, not counted for coverage)"
	if got := fieldLine(t, res.stdout, "plugin PreToolUse"); got != want {
		t.Errorf("plugin PreToolUse reads %q for a hand-edited entry, want %q", got, want)
	}
	if !strings.Contains(res.stdout, "overlap:") {
		t.Errorf("status does not name the overlap with a hand-edited plugin that still fires:\n%s", res.stdout)
	}
}

// TestRubricHLD1_AHandEditedPluginStillRecords: with no store yet, an enabled
// plugin whose groups are hand edited still records the next tool call, so
// recording reads active, not "not installed".
func TestRubricHLD1_AHandEditedPluginStillRecords(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	retimeEveryPluginHook(t, filepath.Join(fx.root, "hooks", "hooks.json"), 30)

	if got := fieldLine(t, e.status().stdout, "recording"); got != "active" {
		t.Errorf("recording reads %q with a hand-edited plugin enabled and no store yet, want active", got)
	}
}

// retimeEveryPluginHook sets the timeout of every hook in every group of a
// plugin's hooks.json, so that intact() accepts none of them.
func retimeEveryPluginHook(t *testing.T, path string, timeout int) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	for _, groups := range doc["hooks"].(map[string]any) {
		for _, g := range groups.([]any) {
			for _, h := range g.(map[string]any)["hooks"].([]any) {
				h.(map[string]any)["timeout"] = timeout
			}
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, path, string(out))
}

// TestRubricHLD1_AHandEditedPluginsRecapPointsAtItsOwnCommand: the recap line
// a hand-edited plugin's own Stop entry prints points at /rashomon:report.
// The plugin is loaded -- Claude Code just ran its binary -- so its slash
// command exists, whatever the entry's timeout says. Under the intact-only
// answer the unmarked invocation read as "not the plugin" and pointed at the
// CLI command instead, the mismatch TestH91_TheReportPointerNamesACommand
// TheOriginHas protects against for an untouched plugin.
func TestRubricHLD1_AHandEditedPluginsRecapPointsAtItsOwnCommand(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	retimeEveryPluginHook(t, filepath.Join(fx.root, "hooks", "hooks.json"), 30)

	run := func(payload string, args ...string) result {
		t.Helper()
		res := e.runBin(fx.bin, payload, args...) // the plugin's entries carry no marker
		if res.exitCode != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, res.exitCode, res.stderr)
		}
		return res
	}
	run(e.sessionPayload("start", testSession), "probe", "start")
	run(defaultPayload().build(t), "hook")
	run(failurePayload(t, testToolUseID, "Exit code 1", false, 30), "post")
	res := run(stopPayload(testSession, "Ran the command as requested.", false), "recap")
	if res.stdout == "" {
		t.Fatal("a turn with a recorded failure and a silent summary printed nothing")
	}
	var out recapOutput
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("recap stdout is not JSON: %v\n%s", err, res.stdout)
	}
	if want := "→ /rashomon:report --session " + testSession; !strings.Contains(out.SystemMessage, want) {
		t.Errorf("line = %q, want it to point at %q: the hand-edited plugin's own Stop entry ran this, "+
			"and its slash command is loaded", out.SystemMessage, want)
	}
}

// TestRubricHLD1_UnreadablePluginHooksReadAsUnreadable: a plugin whose
// hooks.json does not parse reads as "plugin: unreadable" in status, not as
// no plugin installed at all. Before, FindPlugin's inner loop swallowed
// pluginDeclaration's error per event and still returned the plugin, so a
// hooks.json that could not be read or parsed was read as a plugin that
// declares nothing -- the same shape as no plugin at all -- and watch would
// have installed settings entries beside a plugin whose actual state it
// never saw.
func TestRubricHLD1_UnreadablePluginHooksReadAsUnreadable(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	if err := os.WriteFile(filepath.Join(fx.root, "hooks", "hooks.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "plugin: unreadable") {
		t.Errorf("status = %q, want it to say the plugin is unreadable", res.stdout)
	}
	if strings.Contains(res.stdout, "plugin: none installed") {
		t.Errorf("status read an unparseable plugin as no plugin at all:\n%s", res.stdout)
	}

	before := e.settingsBytes()
	if res := e.watch(); res.exitCode == 0 {
		t.Errorf("watch installed beside a plugin whose hooks.json it could not read")
	}
	if !bytes.Equal(before, e.settingsBytes()) {
		t.Errorf("a refusing watch still wrote the settings file")
	}
}
