package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-71 through H-77 -- Part 1 of the Claude Code integration spec: a plugin
// carries the five recorder entries, and ownership becomes a question about
// origin rather than about the install id a plugin manifest cannot carry.
//
// The numbers are provisional from H-71, following the same convention
// H-70's own header names: they are grepped against the merge target
// (H-\(7[1-9]\|9[0-9]\)) before they become the contract, and re-numbered if
// something else claims them first.

// pluginFixture is a rashomon plugin installation built on disk: a copy of
// the binary under test at <root>/bin/rashomon, a manifest naming it, and
// this repository's own plugin/hooks/hooks.json -- not a hand-written
// stand-in, so a test exercises the exact file this repository ships and
// catches drift between that file and what these tests assume it says.
type pluginFixture struct {
	root string
	bin  string
	key  string
}

// newPluginFixture builds the fixture but does not register it anywhere; see
// installed and enable.
func newPluginFixture(t *testing.T, key string) *pluginFixture {
	t.Helper()
	root := t.TempDir()
	bin := copyBinary(t, rashomonBin, filepath.Join(root, "bin", "rashomon"))

	writeJSONFile(t, filepath.Join(root, ".claude-plugin", "plugin.json"),
		`{"name": "rashomon", "defaultEnabled": false}`)

	hooksSrc := filepath.Join(moduleRoot, "plugin", "hooks", "hooks.json")
	hooksBody, err := os.ReadFile(hooksSrc)
	if err != nil {
		t.Fatalf("reading the shipped plugin manifest %s: %v", hooksSrc, err)
	}
	writeJSONFile(t, filepath.Join(root, "hooks", "hooks.json"), string(hooksBody))

	return &pluginFixture{root: root, bin: bin, key: key}
}

// installed writes installed_plugins.json under the environment's plugins
// directory, naming this fixture -- the file install.FindPlugin and
// install.PluginPresent both need to locate a plugin's root from its key.
func (p *pluginFixture) installed(t *testing.T, e *env) {
	t.Helper()
	doc := fmt.Sprintf(`{"version":2,"plugins":{%q:[{"scope":"user","installPath":%q}]}}`, p.key, p.root)
	writeJSONFile(t, filepath.Join(e.configDir, "plugins", "installed_plugins.json"), doc)
}

// enable writes (or clears) this fixture's enabledPlugins entry in
// settings.json, preserving whatever else is already there -- a settings
// install and a plugin enable are two independent edits to one file in the
// scenarios these tests construct.
func (p *pluginFixture) enable(t *testing.T, e *env, on bool) {
	t.Helper()
	doc := map[string]any{}
	if b := e.settingsBytes(); len(b) > 0 {
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("existing settings.json is not valid JSON: %v", err)
		}
	}
	enabled, _ := doc["enabledPlugins"].(map[string]any)
	if enabled == nil {
		enabled = map[string]any{}
	}
	enabled[p.key] = on
	doc["enabledPlugins"] = enabled
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	e.writeSettings(string(out) + "\n")
}

func writeJSONFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// TestH71_PluginOnlyInstallReadsVerified is H-71, the important one: a
// machine with an enabled plugin and NO settings.json entry at all must still
// read verified, because the recorder is genuinely running -- via the
// plugin's own hook binary -- for every phase of this session.
//
// Break: revert internal/hook/coverage.go's Resolve to the settings-only
// install.Present check, and this fails with hook_entry "absent" on every
// record -- the exact failure Part 1 exists to remove.
func TestH71_PluginOnlyInstallReadsVerified(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)

	// No settings.json hook entries anywhere -- the whole point.
	if res := e.runBin(fx.bin, e.sessionPayload("start", testSession), "probe", "start"); res.exitCode != 0 {
		t.Fatalf("probe start via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
		t.Fatalf("hook via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, defaultPost().build(t), "post"); res.exitCode != 0 {
		t.Fatalf("post via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, e.sessionPayload("end", testSession), "probe", "end"); res.exitCode != 0 {
		t.Fatalf("probe end via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	rep := e.report(testSession)
	if rep.Coverage.State != "verified" {
		t.Fatalf("coverage is %s (%v), want verified -- a plugin-only install must read verified",
			rep.Coverage.State, rep.Coverage.Reasons)
	}
	if rep.Coverage.HookEntryAtStart != "present_plugin" || rep.Coverage.HookEntryAtEnd != "present_plugin" {
		t.Errorf("hook entry renders %s/%s, want present_plugin at both ends",
			rep.Coverage.HookEntryAtStart, rep.Coverage.HookEntryAtEnd)
	}

	for _, phase := range []string{"call", "post"} {
		recs := e.coverage(testSession, phase)
		if len(recs) == 0 {
			t.Fatalf("no %s-phase coverage record", phase)
		}
		if got := recs[len(recs)-1].str("hook_entry"); got != "present_plugin" {
			t.Errorf("%s coverage hook_entry is %q, want present_plugin", phase, got)
		}
	}
}

// TestH72_DetachDoesNotTouchPluginEntries is H-72: with both origins present,
// detach removes the settings entries and leaves the plugin's alone -- a
// plugin's entries are not detach's to remove.
//
// Break: point detach at install.FindPlugin (or PluginPresent) instead of the
// settings-only RemoveIf, and detach reports success having removed nothing
// it owns -- or worse, reaches into a file this package does not write.
func TestH72_DetachDoesNotTouchPluginEntries(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)

	hooksBefore, err := os.ReadFile(filepath.Join(fx.root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestBefore, err := os.ReadFile(filepath.Join(fx.root, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}

	if res := e.detach(); res.exitCode != 0 {
		t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := len(ours(e.settings().Hooks["PreToolUse"])); got != 0 {
		t.Errorf("%d settings entries of ours remain after detach", got)
	}

	hooksAfter, err := os.ReadFile(filepath.Join(fx.root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hooksBefore, hooksAfter) {
		t.Errorf("detach modified the plugin's own hooks.json")
	}
	manifestAfter, err := os.ReadFile(filepath.Join(fx.root, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(manifestBefore, manifestAfter) {
		t.Errorf("detach modified the plugin's own manifest")
	}

	// The plugin's own entries still work after detach: self-identification
	// does not depend on anything settings.json carries, so recording through
	// the plugin survives a detach that was aimed at the settings install.
	if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
		t.Fatalf("hook via the plugin binary after detach: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := len(e.declarations(testSession)); got != 1 {
		t.Fatalf("got %d declarations after detach, want 1: the plugin's entry still records", got)
	}
	recs := e.coverage(testSession, "call")
	if got := recs[len(recs)-1].str("hook_entry"); got != "present_plugin" {
		t.Errorf("hook_entry after detach is %q, want present_plugin", got)
	}
}

// TestH73_WatchRefusesUnderALivePlugin is H-73: with the plugin enabled and
// providing the entries, watch exits non-zero, names the plugin, and writes
// nothing.
//
// Break: drop the refusal in cmdWatch and the same session records every
// call twice, which H-74 then catches.
func TestH73_WatchRefusesUnderALivePlugin(t *testing.T) {
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	before := e.settingsBytes()

	res := e.watch()
	if res.exitCode == 0 {
		t.Fatalf("watch installed although the %s plugin already provides these entries", fx.key)
	}
	if !strings.Contains(res.stderr, fx.key) {
		t.Errorf("refusal does not name the plugin %q: %q", fx.key, res.stderr)
	}
	if !strings.Contains(res.stderr, "disable") {
		t.Errorf("refusal does not name the command that removes the other one: %q", res.stderr)
	}
	if !bytes.Equal(before, e.settingsBytes()) {
		t.Errorf("a refusing watch still wrote the settings file")
	}
	if _, err := os.Stat(filepath.Join(e.home, "install.json")); err == nil {
		t.Errorf("a refusing watch still created a store")
	}
}

// TestH74_BothOriginsFiringStillRecordCleanly is H-74: the scenario H-73's
// refusal exists to prevent -- reached here directly, bypassing the CLI
// refusal by constructing both origins as files rather than by calling watch
// while the plugin is live, exactly as the spec's own "migration window"
// tolerates. Two invocations for one Claude-Code-visible tool call is the
// KNOWN, accepted cost of that window (spelled out in "Duplicate
// prevention"); dedup on (session_id, tool_use_id) is explicitly the wrong
// fix, because PostToolUse and PostToolUseFailure legitimately share that id
// (chains.go:215-217). What this item guards is narrower and does not
// depend on resolving that ambiguity: EACH invocation still contributes
// exactly one clean declaration -- neither silently dropped nor written
// twice by a single call -- so the total is exactly the number of
// invocations that actually happened, one per origin.
//
// Break: remove the guard (H-73's refusal) so nothing stops watch from
// running on top of a live plugin, and the resulting session -- both origins
// now firing on every real tool call -- records twice per call instead of
// once per invocation.
func TestH74_BothOriginsFiringStillRecordCleanly(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)

	// One user-visible tool call, as Claude Code would deliver it to BOTH
	// configured entries: the same session and tool_use_id, once through
	// each origin's own command line.
	if res := e.hook(defaultPayload().build(t)); res.exitCode != 0 {
		t.Fatalf("hook via the settings entry: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
		t.Fatalf("hook via the plugin entry: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	decls := e.declarations(testSession)
	if len(decls) != 2 {
		t.Fatalf("got %d declarations for one dual-origin call, want 2 (one per origin, neither dropped nor duplicated): %v",
			len(decls), decls)
	}
	seen := map[string]bool{}
	for _, d := range decls {
		if d.str("tool_use_id") != testToolUseID {
			t.Errorf("declaration carries tool_use_id %q, want %q", d.str("tool_use_id"), testToolUseID)
		}
		seen[d.raw] = true
	}
	if len(seen) != 2 {
		t.Errorf("the two declarations are byte-identical; a real double-write bug would look like this too")
	}
}

// TestH75_StatusNamesBothOrigins is H-75: a settings install and an enabled
// plugin are reported on separate lines, so a user with a stale settings
// entry -- or a stale plugin one -- can tell which is actually live.
//
// Break: collapse them into one line and a user with a stale settings entry
// cannot tell which is recording.
func TestH75_StatusNamesBothOrigins(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// The settings origin's own per-event lines, unchanged from before Part 1.
	if got := fieldLine(t, res.stdout, "PreToolUse"); got != "present" {
		t.Errorf("settings PreToolUse reads %q, want present", got)
	}
	// The plugin origin, on its own distinctly-labelled lines.
	if !strings.Contains(res.stdout, "plugin: "+fx.key+" (enabled)") {
		t.Errorf("status does not name the plugin as enabled:\n%s", res.stdout)
	}
	if got := fieldLine(t, res.stdout, "plugin PreToolUse"); got != "present" {
		t.Errorf("plugin PreToolUse reads %q, want present", got)
	}
	if got := fieldLine(t, res.stdout, "plugin SessionEnd"); got != "present" {
		t.Errorf("plugin SessionEnd reads %q, want present", got)
	}
	if !strings.Contains(res.stdout, "overlap:") || !strings.Contains(res.stdout, "rashomon detach") {
		t.Errorf("status does not name the overlap with rashomon detach as the resolution:\n%s", res.stdout)
	}
}

// TestH75_StatusNamesADisabledOrAbsentPlugin covers the other two plugin
// states status must be able to say, alongside "enabled": a machine with a
// plugin installed but not enabled, and one with none at all. A status that
// only ever said "enabled" whatever it found would pass the test above for
// the wrong reason.
func TestH75_StatusNamesADisabledOrAbsentPlugin(t *testing.T) {
	t.Run("installed but not enabled", func(t *testing.T) {
		e := newEnv(t)
		fx := newPluginFixture(t, "rashomon@test")
		fx.installed(t, e)
		fx.enable(t, e, false)

		res := e.status()
		if res.exitCode != 0 {
			t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if !strings.Contains(res.stdout, "plugin: "+fx.key+" (disabled)") {
			t.Errorf("status does not name the plugin as disabled:\n%s", res.stdout)
		}
		if strings.Contains(res.stdout, "overlap:") {
			t.Errorf("status reports an overlap when the plugin is disabled:\n%s", res.stdout)
		}
	})

	t.Run("none installed", func(t *testing.T) {
		e := newEnv(t)
		res := e.status()
		if res.exitCode != 0 {
			t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if !strings.Contains(res.stdout, "plugin: none installed") {
			t.Errorf("status does not say no plugin is installed:\n%s", res.stdout)
		}
	})
}

// TestH76_SkillsResolveThePluginBinaryFirst is H-76: the plugin's own copies
// of the five skills point straight at ${CLAUDE_PLUGIN_ROOT}/bin/rashomon,
// with no PATH lookup and no search across the three locations the
// standalone skills under skills/ still carry.
//
// Break: leave the three-path preamble in the plugin's copies, and a machine
// with an old ~/go/bin/rashomon runs the wrong binary from the plugin's own
// command.
func TestH76_SkillsResolveThePluginBinaryFirst(t *testing.T) {
	names := []string{"report", "status", "stop", "forget", "watch"}
	dir := filepath.Join(moduleRoot, "plugin", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	got := map[string]bool{}
	for _, en := range entries {
		got[en.Name()] = true
	}
	for _, name := range names {
		if !got[name] {
			t.Errorf("plugin/skills is missing %q", name)
		}
	}

	for _, name := range names {
		path := filepath.Join(dir, name, "SKILL.md")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		text := string(body)
		if !strings.Contains(text, "${CLAUDE_PLUGIN_ROOT}/bin/rashomon") {
			t.Errorf("%s does not resolve the plugin-local binary", path)
		}
		for _, stale := range []string{"~/.local/bin", "~/go/bin", "command -v rashomon", "/opt/homebrew/bin"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still carries the standalone three-path preamble (%q); "+
					"bin/ is on PATH for a plugin, and a stale search can resolve the wrong binary", path, stale)
			}
		}
	}
}

// TestH77_ManifestShipsDisabled is H-77: defaultEnabled is false, and a fresh
// install -- present on disk, named in no settings scope -- records nothing
// and does not block a settings-based watch either, because there is nothing
// live to collide with.
//
// Break: set defaultEnabled true (or drop the field, whose documented default
// is true) and installation starts recording, contradicting the amended
// third constraint: "Installation does not start recording without
// enablement."
func TestH77_ManifestShipsDisabled(t *testing.T) {
	manifestPath := filepath.Join(moduleRoot, "plugin", ".claude-plugin", "plugin.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading %s: %v", manifestPath, err)
	}
	var m struct {
		Name           string `json:"name"`
		DefaultEnabled *bool  `json:"defaultEnabled"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("%s is not valid JSON: %v", manifestPath, err)
	}
	if m.Name != "rashomon" {
		t.Errorf("manifest name is %q, want \"rashomon\"", m.Name)
	}
	if m.DefaultEnabled == nil {
		t.Fatalf("manifest omits defaultEnabled, whose documented default is true -- installation would start recording")
	}
	if *m.DefaultEnabled {
		t.Errorf("manifest sets defaultEnabled true; installation would start recording without enablement")
	}

	// A fresh install: present on disk, present in installed_plugins.json,
	// named in NO settings scope at all -- the state a plugin actually
	// reaches the moment it is added and before anyone has touched it.
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "plugin: "+fx.key+" (disabled)") {
		t.Errorf("a fresh, untouched install does not read disabled:\n%s", res.stdout)
	}

	// Nothing live to collide with, so a settings-based watch proceeds
	// normally rather than refusing.
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch refused with only a disabled plugin present: exit %d, stderr %q", res.exitCode, res.stderr)
	}
}
